import importlib.util
import json
from pathlib import Path
import unittest

spec=importlib.util.spec_from_file_location('s3prepare',Path(__file__).with_name('prepare.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)

class PlanSafety(unittest.TestCase):
    def test_full_fabric_requires_three_fixed_distinct_zone_groups(self):
        v=m.template();r=v['Resources']
        for z in 'ABC':
            node=r['Nodes'+z]['Properties']
            self.assertEqual(node['Subnets'],[m.ref('Subnet'+z)])
            self.assertEqual(node['ScalingConfig'],{'MinSize':1,'MaxSize':1,'DesiredSize':1})
            self.assertEqual(node['CapacityType'],'ON_DEMAND')
        self.assertNotIn('Condition',r['NodesA'])
        self.assertEqual(r['NodesB']['Condition'],'FullFabric')
        self.assertEqual(r['NodesC']['Condition'],'FullFabric')
    def test_no_retained_vpc_or_costly_extra_service(self):
        v=m.template();types={x['Type'] for x in v['Resources'].values()}
        self.assertNotIn('AWS::EC2::NatGateway',types)
        self.assertNotIn('AWS::ElasticLoadBalancingV2::LoadBalancer',types)
        self.assertNotIn('AWS::RDS::DBInstance',types)
        self.assertEqual(v['Resources']['VPC']['Type'],'AWS::EC2::VPC')
        self.assertEqual(v['Resources']['Cluster']['Properties']['Version'],'1.34')
        self.assertFalse(v['Resources']['Cluster']['Properties']['AccessConfig']['BootstrapClusterCreatorAdminPermissions'])
    def test_storage_and_enforcing_network_policy(self):
        r=m.template()['Resources'];disk=r['NodeTemplate']['Properties']['LaunchTemplateData']['BlockDeviceMappings'][0]['Ebs']
        self.assertTrue(disk['Encrypted']);self.assertTrue(disk['DeleteOnTermination']);self.assertEqual(disk['VolumeType'],'gp3')
        cni=json.loads(r['VPCNetworking']['Properties']['ConfigurationValues'])
        self.assertEqual(cni['enableNetworkPolicy'],'true');self.assertEqual(cni['env']['NETWORK_POLICY_ENFORCING_MODE'],'strict')
        self.assertTrue(r['Cluster']['Properties']['ResourcesVpcConfig']['EndpointPrivateAccess'])
        self.assertEqual(r['Cluster']['Properties']['ResourcesVpcConfig']['PublicAccessCidrs'],[m.ref('OperatorCIDR')])
    def test_root_wrong_account_shared_name_and_broad_network_rejected(self):
        base=['sathonate-s3-1234abcd','arn:aws:iam::429488690613:role/SathonateTrial','8.8.8.8/32','FullFabric']
        for i,value in [(0,'smp-qualification'),(1,'arn:aws:iam::429488690613:root'),(1,'arn:aws:iam::111111111111:role/Trial'),(2,'0.0.0.0/0'),(2,'10.0.0.1/32'),(3,'production')]:
            args=base.copy();args[i]=value
            with self.assertRaises(ValueError):m.validate_parameters(*args)
        self.assertEqual(len(m.validate_parameters(*base)),4)
    def test_all_resource_references_exist(self):
        v=m.template();known=set(v['Resources'])|set(v['Parameters'])
        def walk(x):
            if isinstance(x,dict):
                if 'Ref' in x:self.assertIn(x['Ref'],known)
                if 'Fn::GetAtt' in x:self.assertIn(x['Fn::GetAtt'][0],v['Resources'])
                for y in x.values():walk(y)
            elif isinstance(x,list):
                for y in x:walk(y)
        walk(v)

if __name__=='__main__':unittest.main()
