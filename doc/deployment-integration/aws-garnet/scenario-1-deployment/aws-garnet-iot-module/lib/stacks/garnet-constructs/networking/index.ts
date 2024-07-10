import { CfnSubnet, Port, SecurityGroup, SubnetType, Vpc } from "aws-cdk-lib/aws-ec2"
import { Construct } from "constructs"
import { Parameters } from "../../../../parameters"
import { Aws, CfnOutput, Fn, Lazy, Names, Stack } from "aws-cdk-lib";

export interface GarnetNetworkingProps {
  az1: string,
  az2: string
}

export class GarnetNetworking extends Construct {
  public readonly vpc: Vpc
  constructor(scope: Construct, id: string, props: GarnetNetworkingProps) {
    super(scope, id)
    const stack = Stack.of(this)
    let broker_id = Parameters.garnet_broker
    // VPC
    const vpc = new Vpc(this, `VpcGarnet${broker_id}`, {
      natGateways: 1,
      availabilityZones: [`${props.az1}`,`${props.az2}`],
      vpcName: `garnet-vpc-${broker_id.toLowerCase()}-${Names.uniqueId(this).slice(-4).toLowerCase()}`,
      subnetConfiguration: [
        {
          subnetType: SubnetType.PRIVATE_WITH_EGRESS,
          name: `garnet-subnet-egress-${broker_id.toLowerCase()}`,
        },
        {
          subnetType: SubnetType.PRIVATE_ISOLATED,
          name: `garnet-subnet-isolated-${broker_id.toLowerCase()}`,
        },
        {
          subnetType: SubnetType.PUBLIC,
          name: `garnet-subnet-public-${broker_id.toLowerCase()}`,
        },
      ],
    })


    this.vpc = vpc;
  }
}
