"""Offline two-phase CloudFormation plan using administrator-owned IAM roles."""
import argparse
import json
from pathlib import Path

from prepare import template

RUN = 'sathonate-s3-20261027'
ACCOUNT = '429488690613'


def admin_owned_template():
    doc = template()
    resources = doc['Resources']
    roles = {key: f'arn:aws:iam::{ACCOUNT}:role/{RUN}-{key.lower()}'
             for key in ['ClusterRole', 'NodeRole', 'CNIRole', 'EBSRole']}
    for key in [*roles, 'OIDCProvider']:
        del resources[key]

    def replace(value):
        if isinstance(value, dict):
            target = value.get('Fn::GetAtt')
            if isinstance(target, list) and target[0] in roles and target[1] == 'Arn':
                return roles[target[0]]
            return {k: replace(v) for k, v in value.items()}
        if isinstance(value, list):
            return [replace(v) for v in value]
        return value

    doc = replace(doc)
    resources = doc['Resources']
    parameters = doc['Parameters']
    parameters['ClusterName'] = {'Type': 'String', 'AllowedValues': [RUN], 'Default': RUN}
    parameters['Mode'] = {'Type': 'String', 'AllowedValues': ['FullFabric'], 'Default': 'FullFabric'}
    parameters['EnableWorkloads'] = {'Type': 'String', 'AllowedValues': ['false', 'true'], 'Default': 'false',
        'Description': 'Keep false until an administrator has bound both addon role trusts to this cluster issuer.'}
    parameters['OperatorRoleArn']['AllowedPattern'] = (
        r'^arn:aws:iam::429488690613:role/aws-reserved/sso\.amazonaws\.com/'
        r'([a-z]{2}(-[a-z]+)+-[0-9]+/)?AWSReservedSSO_Sprint3-permissions_[a-fA-F0-9]{16}$')
    doc['Conditions']['WorkloadsReady'] = {'Fn::Equals': [{'Ref': 'EnableWorkloads'}, 'true']}
    doc['Conditions']['FullFabric'] = {'Fn::And': [
        {'Condition': 'WorkloadsReady'}, {'Fn::Equals': [{'Ref': 'Mode'}, 'FullFabric']}]}
    resources['Cluster']['Properties']['UpgradePolicy'] = {'SupportType': 'STANDARD'}
    for key in ['VPCNetworking', 'NodeTemplate', 'NodesA', 'DNS', 'Proxy', 'StorageDriver']:
        resources[key]['Condition'] = 'WorkloadsReady'
    doc['Outputs']['OIDCIssuer'] = {'Value': {'Fn::GetAtt': ['Cluster', 'OpenIdConnectIssuerUrl']}}
    doc['Description'] = 'Isolated Sprint 3: admin-owned IAM; cluster-only default; no AWS calls made by this generator.'
    return doc


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    doc = admin_owned_template()
    args.output.mkdir(parents=True, exist_ok=False)
    (args.output / 'infrastructure-admin-owned.json').write_text(json.dumps(doc, indent=2) + '\n')
    plan = {'status': 'LOCAL_PREPARATION_ONLY', 'run': RUN, 'region': 'ap-south-1',
        'requiredVCPU': 6, 'maximumHours': 4, 'maximumUSD': 20,
        'cloudActionsPerformed': False, 'productionEnabled': False,
        'phases': ['EnableWorkloads=false: cluster and explicit operator access only',
                   'Administrator: exact issuer OIDC and addon trust setup',
                   'EnableWorkloads=true: three-AZ nodes and addons'],
        'blockers': ['Authenticated approved non-root SSO profile and live dependencies unverified',
                     'AWS CloudFormation validation not executed',
                     'Secure application-controlled API-to-EKS adapter not yet qualified',
                     'No automatic teardown executor qualified; do not start infrastructure awaiting input']}
    (args.output / 'plan.json').write_text(json.dumps(plan, indent=2) + '\n')
    print('Offline administrator-owned plan saved; no AWS action performed')


if __name__ == '__main__':
    main()
