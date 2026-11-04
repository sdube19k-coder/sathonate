"""Static checks against public AWS service-reference JSON; no account API calls."""
import concurrent.futures
import argparse
import hashlib
import json
from pathlib import Path
import re
import urllib.request

HERE = Path(__file__).resolve().parent
POLICY = HERE / 'sprint3-identity-center-policy.json'


def values(value):
    return value if isinstance(value, list) else [value]


def matches(template, value):
    expression = re.escape(template).replace(re.escape('${TagKey}'), '.+')
    return re.fullmatch(expression, value, flags=re.IGNORECASE) is not None


def fetch(service):
    url = f'https://servicereference.us-east-1.amazonaws.com/v1/{service}/{service}.json'
    with urllib.request.urlopen(url, timeout=25) as response:
        return service, json.load(response)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--policy', type=Path, default=POLICY)
    parser.add_argument('--report', type=Path, default=HERE / 'sprint3-policy-validation.json')
    args = parser.parse_args()
    raw = args.policy.read_bytes()
    policy = json.loads(raw)
    errors, broad, coverage = [], [], []
    assert policy['Version'] == '2012-10-17'
    statements = policy['Statement']
    compact = len(json.dumps(policy, separators=(',', ':')).encode())
    if compact > 10240 or len(raw) > 32768:
        errors.append('Identity Center inline policy size exceeded')
    services = sorted({a.split(':')[0] for s in statements for a in values(s['Action'])})
    with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
        references = dict(pool.map(fetch, services))
    for index, statement in enumerate(statements):
        assert statement['Effect'] == 'Allow'
        for resource in values(statement['Resource']):
            if resource != '*' and not re.match(r'^arn:aws:[a-z0-9-]+:[a-z0-9-]*:(429488690613|aws|):.+$', resource):
                errors.append(f'{index}: malformed/unapproved account ARN: {resource}')
        for action in values(statement['Action']):
            service, name = action.split(':')
            ref = references[service]
            entry = next((a for a in ref['Actions'] if a['Name'].lower() == name.lower()), None)
            if not entry or '*' in action:
                errors.append(f'{index}: invalid/wildcard action {action}')
                continue
            resources = {r['Name']: r for r in ref.get('Resources', [])}
            action_resources = entry.get('Resources', [])
            keys = set(entry.get('ActionConditionKeys', []))
            for resource in action_resources:
                keys.update(resource.get('ConditionKeys', []))
                keys.update(resources.get(resource['Name'], {}).get('ConditionKeys', []))
            for operator, conditions in statement.get('Condition', {}).items():
                if operator not in {'StringEquals', 'ArnLike', 'Null', 'Bool'}:
                    errors.append(f'{index}: unreviewed operator {operator}')
                for key, val in conditions.items():
                    if key != 'aws:RequestedRegion' and not any(matches(k, key) for k in keys):
                        errors.append(f'{index}: condition {key} unsupported for {action}')
                    types = {t for k in ref.get('ConditionKeys', []) if matches(k['Name'], key) for t in k.get('Types', [])}
                    if operator == 'Bool' and types and types != {'Bool'}:
                        errors.append(f'{index}: Bool used with {key}: {sorted(types)}')
            if statement['Resource'] == '*':
                broad.append({'action': action, 'resourceTypes': [r['Name'] for r in action_resources]})
                if action_resources:
                    errors.append(f'{index}: unnecessary literal Resource * for {action}')
            elif action_resources:
                # Match ARN service, regional/account binding and resource kind.
                # Wildcard suffixes are allowed only after that checked prefix.
                allowed_types = {r['Name'] for r in action_resources}
                found = False
                for resource in values(statement['Resource']):
                    for kind in allowed_types:
                        for fmt in resources.get(kind, {}).get('ARNFormats', []):
                            prefix = fmt.split(':', 5)
                            target = resource.split(':', 5)
                            if len(prefix) != 6 or len(target) != 6 or target[2] != service:
                                continue
                            fixed = prefix[5].split('${', 1)[0]
                            if not fixed or target[5].startswith(fixed):
                                found = True
                if not found:
                    errors.append(f'{index}: no compatible resource ARN for {action}')
            coverage.append({'statement': index, 'action': action, 'resourceTypes': [r['Name'] for r in action_resources], 'conditionKeys': sorted(keys)})
    iam = [s for s in statements if any(a.startswith('iam:') for a in values(s['Action']))]
    approved = {f'arn:aws:iam::429488690613:role/sathonate-s3-20261027-{x}' for x in ['clusterrole', 'noderole', 'cnirole', 'ebsrole']}
    for statement in iam:
        if not set(values(statement['Action'])) <= {'iam:GetRole', 'iam:PassRole'} or not set(values(statement['Resource'])) <= approved:
            errors.append('IAM escaped approved role/action set')
        if 'iam:PassRole' in values(statement['Action']) and 'iam:PassedToService' not in statement.get('Condition', {}).get('StringEquals', {}):
            errors.append('PassRole lacks service restriction')
    result = {'status': 'PASS' if not errors else 'FAIL', 'scope': 'Static JSON, size, action, condition-key, ARN-kind and IAM-boundary checks using public AWS references. Not a live authorization or deployment acceptance.', 'policySHA256': hashlib.sha256(raw).hexdigest(), 'nonWhitespaceBytes': compact, 'fileBytes': len(raw), 'errors': errors, 'literalStarActions': broad, 'actionCoverage': coverage, 'liveAccessAnalyzer': 'NOT_EXECUTED', 'liveSimulation': 'NOT_EXECUTED'}
    args.report.write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps({k: result[k] for k in ['status', 'nonWhitespaceBytes', 'fileBytes', 'errors']}))
    if errors:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
