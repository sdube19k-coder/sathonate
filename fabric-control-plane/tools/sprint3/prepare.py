"""Offline infrastructure plan. No AWS SDK, shell command, credential or quota call."""
import argparse
import hashlib
import ipaddress
import json
from pathlib import Path
import re


def ref(name): return {'Ref': name}
def sub(value): return {'Fn::Sub': value}
def attr(name, key): return {'Fn::GetAtt': [name, key]}


def template():
    resources = {}
    def add(name, kind, props, **extra):
        resources[name] = {'Type': 'AWS::' + kind, 'Properties': props, **extra}
    tags = [{'Key': 'SathonateRun', 'Value': ref('ClusterName')}]
    add('VPC', 'EC2::VPC', {'CidrBlock': '10.77.0.0/16', 'EnableDnsSupport': True, 'EnableDnsHostnames': True, 'Tags': tags})
    add('InternetGateway', 'EC2::InternetGateway', {'Tags': tags})
    add('GatewayAttachment', 'EC2::VPCGatewayAttachment', {'VpcId': ref('VPC'), 'InternetGatewayId': ref('InternetGateway')})
    add('RouteTable', 'EC2::RouteTable', {'VpcId': ref('VPC'), 'Tags': tags})
    add('InternetRoute', 'EC2::Route', {'RouteTableId': ref('RouteTable'), 'DestinationCidrBlock': '0.0.0.0/0', 'GatewayId': ref('InternetGateway')}, DependsOn='GatewayAttachment')
    for i, zone in enumerate('ABC'):
        add('Subnet' + zone, 'EC2::Subnet', {'VpcId': ref('VPC'), 'AvailabilityZone': ref('Zone' + zone), 'CidrBlock': f'10.77.{i}.0/24', 'MapPublicIpOnLaunch': True, 'Tags': tags})
        add('Route' + zone, 'EC2::SubnetRouteTableAssociation', {'SubnetId': ref('Subnet' + zone), 'RouteTableId': ref('RouteTable')})
    def role(name, service, policies):
        add(name, 'IAM::Role', {'RoleName': sub('${ClusterName}-' + name.lower()), 'AssumeRolePolicyDocument': {'Version': '2012-10-17', 'Statement': [{'Effect': 'Allow', 'Principal': {'Service': service}, 'Action': 'sts:AssumeRole'}]}, 'ManagedPolicyArns': [sub('arn:${AWS::Partition}:iam::aws:policy/' + p) for p in policies], 'Tags': tags})
    role('ClusterRole', 'eks.amazonaws.com', ['AmazonEKSClusterPolicy'])
    role('NodeRole', 'ec2.amazonaws.com', ['AmazonEKSWorkerNodePolicy', 'AmazonEC2ContainerRegistryPullOnly'])
    add('AuditLogs', 'Logs::LogGroup', {'LogGroupName': sub('/aws/eks/${ClusterName}/cluster'), 'RetentionInDays': 1, 'Tags': tags})
    add('Cluster', 'EKS::Cluster', {'Name': ref('ClusterName'), 'Version': '1.34', 'RoleArn': attr('ClusterRole', 'Arn'),
        'BootstrapSelfManagedAddons': False,
        'AccessConfig': {'AuthenticationMode': 'API', 'BootstrapClusterCreatorAdminPermissions': False},
        'ResourcesVpcConfig': {'SubnetIds': [ref('Subnet' + z) for z in 'ABC'], 'EndpointPrivateAccess': True, 'EndpointPublicAccess': True, 'PublicAccessCidrs': [ref('OperatorCIDR')]},
        'Logging': {'ClusterLogging': {'EnabledTypes': [{'Type': 'audit'}, {'Type': 'authenticator'}]}}, 'Tags': tags}, DependsOn='AuditLogs')
    add('OperatorAccess', 'EKS::AccessEntry', {'ClusterName': ref('Cluster'), 'PrincipalArn': ref('OperatorRoleArn'), 'Type': 'STANDARD', 'AccessPolicies': [{'PolicyArn': 'arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy', 'AccessScope': {'Type': 'cluster'}}], 'Tags': tags})
    add('OIDCProvider', 'IAM::OIDCProvider', {'Url': attr('Cluster', 'OpenIdConnectIssuerUrl'), 'ClientIdList': ['sts.amazonaws.com'], 'Tags': tags})
    for name, sa, policy in [('CNIRole', 'aws-node', 'AmazonEKS_CNI_Policy'), ('EBSRole', 'ebs-csi-controller-sa', 'service-role/AmazonEBSCSIDriverPolicy')]:
        trust = {'Fn::Sub': [json.dumps({'Version': '2012-10-17', 'Statement': [{'Effect': 'Allow', 'Principal': {'Federated': '${OIDCProvider}'}, 'Action': 'sts:AssumeRoleWithWebIdentity', 'Condition': {'StringEquals': {'${Issuer}:aud': 'sts.amazonaws.com', '${Issuer}:sub': 'system:serviceaccount:kube-system:' + sa}}}]}), {'Issuer': {'Fn::Select': [1, {'Fn::Split': ['https://', attr('Cluster', 'OpenIdConnectIssuerUrl')]}]}}]}
        add(name, 'IAM::Role', {'RoleName': sub('${ClusterName}-' + name.lower()), 'AssumeRolePolicyDocument': trust, 'ManagedPolicyArns': [sub('arn:${AWS::Partition}:iam::aws:policy/' + policy)], 'Tags': tags})
    add('VPCNetworking', 'EKS::Addon', {'ClusterName': ref('Cluster'), 'AddonName': 'vpc-cni', 'ServiceAccountRoleArn': attr('CNIRole', 'Arn'), 'ConfigurationValues': json.dumps({'enableNetworkPolicy': 'true', 'env': {'NETWORK_POLICY_ENFORCING_MODE': 'strict'}}), 'Tags': tags})
    add('NodeTemplate', 'EC2::LaunchTemplate', {'LaunchTemplateData': {'MetadataOptions': {'HttpTokens': 'required', 'HttpPutResponseHopLimit': 2}, 'BlockDeviceMappings': [{'DeviceName': '/dev/xvda', 'Ebs': {'Encrypted': True, 'VolumeType': 'gp3', 'VolumeSize': 30, 'DeleteOnTermination': True}}], 'TagSpecifications': [{'ResourceType': 'instance', 'Tags': tags}, {'ResourceType': 'volume', 'Tags': tags}]}, 'TagSpecifications': [{'ResourceType': 'launch-template', 'Tags': tags}]})
    for zone in 'ABC':
        add('Nodes' + zone, 'EKS::Nodegroup', {'ClusterName': ref('Cluster'), 'NodegroupName': 'zone-' + zone.lower(), 'NodeRole': attr('NodeRole', 'Arn'), 'Subnets': [ref('Subnet' + zone)], 'InstanceTypes': ['m7i-flex.large'], 'AmiType': 'AL2023_x86_64_STANDARD', 'CapacityType': 'ON_DEMAND', 'ScalingConfig': {'MinSize': 1, 'MaxSize': 1, 'DesiredSize': 1}, 'LaunchTemplate': {'Id': ref('NodeTemplate'), 'Version': attr('NodeTemplate', 'LatestVersionNumber')}, 'UpdateConfig': {'MaxUnavailable': 1}, 'Tags': {'SathonateRun': ref('ClusterName')}}, DependsOn=['VPCNetworking', 'InternetRoute'], **({'Condition': 'FullFabric'} if zone != 'A' else {}))
    for name, addon in [('DNS', 'coredns'), ('Proxy', 'kube-proxy'), ('StorageDriver', 'aws-ebs-csi-driver')]:
        props = {'ClusterName': ref('Cluster'), 'AddonName': addon, 'Tags': tags}
        if name == 'StorageDriver': props['ServiceAccountRoleArn'] = attr('EBSRole', 'Arn')
        add(name, 'EKS::Addon', props, DependsOn='NodesA')
    return {'AWSTemplateFormatVersion': '2010-09-09', 'Description': 'New isolated Sathonate qualification only; no retained VPC or credentials. No Auto Mode, NAT or LB.',
        'Parameters': {'ClusterName': {'Type': 'String', 'AllowedPattern': '^sathonate-s3-[a-f0-9]{8}$'}, 'OperatorRoleArn': {'Type': 'String', 'AllowedPattern': '^arn:aws:iam::429488690613:role/[A-Za-z0-9+=,.@_/-]+$'}, 'OperatorCIDR': {'Type': 'String', 'AllowedPattern': r'^(\d{1,3}\.){3}\d{1,3}/32$'}, 'Mode': {'Type': 'String', 'AllowedValues': ['FullFabric', 'InfrastructureSmoke'], 'Default': 'FullFabric'}, **{'Zone'+z: {'Type': 'AWS::EC2::AvailabilityZone::Name', 'Default': 'ap-south-1'+z.lower()} for z in 'ABC'}},
        'Conditions': {'FullFabric': {'Fn::Equals': [ref('Mode'), 'FullFabric']}}, 'Resources': resources,
        'Outputs': {'Cluster': {'Value': ref('Cluster')}, 'VPC': {'Value': ref('VPC')}, 'Endpoint': {'Value': attr('Cluster', 'Endpoint')}, 'Role': {'Value': ref('OperatorRoleArn')}, 'LogGroup': {'Value': ref('AuditLogs')}}}


def validate_parameters(cluster, role, cidr, mode):
    if not re.fullmatch(r'sathonate-s3-[a-f0-9]{8}', cluster): raise ValueError('fresh run-specific cluster name required')
    if not re.fullmatch(r'arn:aws:iam::429488690613:role/[A-Za-z0-9+=,.@_/-]+', role): raise ValueError('approved non-root role in account required')
    ip = ipaddress.ip_network(cidr, strict=True)
    if ip.version != 4 or ip.prefixlen != 32 or not ip.network_address.is_global: raise ValueError('approved public operator IPv4 /32 required')
    if mode not in ('FullFabric', 'InfrastructureSmoke'): raise ValueError('unsupported mode')
    return [{'ParameterKey': k, 'ParameterValue': v} for k,v in [('ClusterName',cluster),('OperatorRoleArn',role),('OperatorCIDR',cidr),('Mode',mode)]]


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--output', type=Path, required=True)
    p.add_argument('--cluster')
    p.add_argument('--operator-role-arn')
    p.add_argument('--operator-cidr')
    p.add_argument('--mode', choices=['FullFabric','InfrastructureSmoke'], default='FullFabric')
    args = p.parse_args()
    args.output.mkdir(parents=True, exist_ok=False)
    raw = (json.dumps(template(), indent=2)+'\n').encode()
    (args.output/'infrastructure.json').write_bytes(raw)
    plan = {'status':'LOCAL_PREPARATION_ONLY','accountEnding':'0613','region':'ap-south-1','templateSHA256':hashlib.sha256(raw).hexdigest(),'mode':'FullFabric','requiredVCPU':6,'recommendedQuota':8,'smokeVCPU':2,'fiveVCPUConclusion':'Infrastructure smoke only; not the supported three-AZ Fabric qualification','maximumHours':4,'proposedMaximumUSD':20,'productionEnabled':False,'rootExecutionAllowed':False,'cloudActionsPerformed':False,'scope':'Standalone protected-issuer/controller compatibility fixture on EKS. Not application API-to-EKS or Sprint 2 closure.','blockers':['Billing verification and administrator bootstrap required','Existing non-root role and operator /32 must be supplied','FullFabric requires sufficient applied quota','Real CNI/EBS/registry/transactions and cleanup only pass after actual cloud execution']}
    (args.output/'plan.json').write_text(json.dumps(plan,indent=2)+'\n',encoding='utf-8')
    if any((args.cluster,args.operator_role_arn,args.operator_cidr)):
        params=validate_parameters(args.cluster or '',args.operator_role_arn or '',args.operator_cidr or '',args.mode)
        (args.output/'parameters.json').write_text(json.dumps(params,indent=2)+'\n',encoding='utf-8')
    print('Offline plan generated; no AWS/Kubernetes action performed')


if __name__ == '__main__': main()
