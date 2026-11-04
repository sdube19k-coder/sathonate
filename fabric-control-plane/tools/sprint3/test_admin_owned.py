import json
import re
import unittest

from prepare_admin_owned import ACCOUNT, RUN, admin_owned_template


class AdminOwnedPlan(unittest.TestCase):
    def test_no_inline_iam_and_exact_roles(self):
        doc = admin_owned_template()
        self.assertFalse(any(v['Type'].startswith('AWS::IAM::') for v in doc['Resources'].values()))
        raw = json.dumps(doc)
        for role in ['clusterrole', 'noderole', 'cnirole', 'ebsrole']:
            self.assertIn(f'arn:aws:iam::{ACCOUNT}:role/{RUN}-{role}', raw)
        for old in ['ClusterRole', 'NodeRole', 'CNIRole', 'EBSRole', 'OIDCProvider']:
            self.assertNotIn('"Fn::GetAtt": ["' + old + '"', raw)

    def test_default_creates_no_nodes_or_addons(self):
        doc = admin_owned_template()
        self.assertEqual(doc['Parameters']['EnableWorkloads']['Default'], 'false')
        for resource in doc['Resources'].values():
            if resource['Type'] in ['AWS::EKS::Addon', 'AWS::EKS::Nodegroup', 'AWS::EC2::LaunchTemplate']:
                self.assertIn(resource['Condition'], ['WorkloadsReady', 'FullFabric'])
        self.assertEqual(doc['Conditions']['FullFabric']['Fn::And'][0], {'Condition': 'WorkloadsReady'})

    def test_exact_sso_binding_and_standard_support(self):
        doc = admin_owned_template()
        pattern = doc['Parameters']['OperatorRoleArn']['AllowedPattern']
        allowed = f'arn:aws:iam::{ACCOUNT}:role/aws-reserved/sso.amazonaws.com/ap-south-1/AWSReservedSSO_Sprint3-permissions_0123456789abcdef'
        self.assertRegex(allowed, pattern)
        self.assertIsNone(re.fullmatch(pattern, allowed.replace('Sprint3-permissions', 'OtherPermissionSet')))
        self.assertIsNone(re.fullmatch(pattern, f'arn:aws:iam::{ACCOUNT}:root'))
        self.assertEqual(doc['Resources']['Cluster']['Properties']['UpgradePolicy']['SupportType'], 'STANDARD')
        self.assertEqual(doc['Parameters']['ClusterName']['AllowedValues'], [RUN])

    def test_references_and_dependencies_exist(self):
        doc = admin_owned_template()
        names = set(doc['Resources']) | set(doc['Parameters'])
        def check(v):
            if isinstance(v, list):
                for x in v: check(x)
            if not isinstance(v, dict): return
            if 'Ref' in v: self.assertTrue(v['Ref'] in names or v['Ref'].startswith('AWS::'))
            if 'Fn::GetAtt' in v: self.assertIn(v['Fn::GetAtt'][0], doc['Resources'])
            for x in v.values(): check(x)
        check(doc)
        for resource in doc['Resources'].values():
            deps = resource.get('DependsOn', [])
            for dep in ([deps] if isinstance(deps, str) else deps):
                self.assertIn(dep, doc['Resources'])


if __name__ == '__main__':
    unittest.main()
