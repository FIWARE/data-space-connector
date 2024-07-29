import { NestedStack, NestedStackProps } from "aws-cdk-lib";
import { Construct } from "constructs";
import { GarnetSecret } from "./secret";
import { GarnetNetworking } from "./networking";
import { Utils } from "./utils";
import { Vpc } from "aws-cdk-lib/aws-ec2";
import { Secret } from "aws-cdk-lib/aws-secretsmanager";
import { GarnetBucket } from "./bucket";
import { CfnDeliveryStream } from "aws-cdk-lib/aws-kinesisfirehose";


export class GarnetConstructs extends NestedStack {
  public readonly vpc: Vpc
  public readonly secret: Secret
  public readonly bucket_name: string
  public readonly az1: string
  public readonly az2: string

  constructor(scope: Construct, id: string, props?: NestedStackProps) {
    super(scope, id, props);
    
    const utils_construct = new Utils(this, "Utils")
    const bucket_construct = new GarnetBucket(this, 'Bucket', {})
    const secret_construct = new GarnetSecret(this, "Secret", {})
    const networking_construct = new GarnetNetworking(this, "Networking", {
      az1: utils_construct.az1,
      az2: utils_construct.az2
    })

    networking_construct.node.addDependency(utils_construct)
    

    this.az1 = utils_construct.az1,
    this.az2 = utils_construct.az2
    this.vpc = networking_construct.vpc
    this.secret = secret_construct.secret 
    this.bucket_name = bucket_construct.bucket_name

  }
}
