import { Aws, CfnOutput, NestedStack, NestedStackProps } from "aws-cdk-lib";
import { Vpc } from "aws-cdk-lib/aws-ec2";
import { Construct } from "constructs";
import { Parameters } from "../../../parameters";
import { GarnetApiGateway } from "../garnet-constructs/apigateway";
//import { GarnetScorpioDatabase } from "./database";
//import { GarnetScorpioFargate } from "./fargate";
import { GarnetNetworking } from "../garnet-constructs/networking";
import { GarnetSecret } from "../garnet-constructs/secret";
import { Secret } from "aws-cdk-lib/aws-secretsmanager";


export interface GarnetScorpioProps extends NestedStackProps{
  vpc: Vpc,
  secret: Secret
}

export class GarnetScorpio extends NestedStack {
  public readonly dns_context_broker: string;
  public readonly vpc: Vpc;
  public readonly broker_api_endpoint: string;
  public readonly api_ref: string;

  constructor(scope: Construct, id: string, props: GarnetScorpioProps) {
    super(scope, id, props);

    // const database_construct = new GarnetScorpioDatabase(this, "Database", {
    //   vpc: props.vpc,
    //   secret_arn: props.secret.secretArn,
    // });

    // const fargate_construct = new GarnetScorpioFargate(
    //   this,
    //   "Fargate",
    //   {
    //     vpc: props.vpc,
    //     sg_database: database_construct.sg_database,
    //     secret_arn: props.secret.secretArn,
    //     db_endpoint: database_construct.database_endpoint,
    //     db_port: database_construct.database_port,
    //     image_context_broker: Parameters.garnet_scorpio.image_context_broker,
    //   }
    // );

    // fargate_construct.node.addDependency(database_construct)

    const api_stack = new GarnetApiGateway(this, "Api", {
      vpc: props.vpc,
      fargate_albListenerArn: Parameters.amazon_eks_cluster_load_balancer_listener_arn,//fargate_construct.fargate_alb,
    });

    new CfnOutput(this, "garnet_endpoint", {
      value: `https://${api_stack.api_ref}.execute-api.${Aws.REGION}.amazonaws.com`,
    });

    this.broker_api_endpoint = `https://${api_stack.api_ref}.execute-api.${Aws.REGION}.amazonaws.com`;
    this.dns_context_broker =
      Parameters.amazon_eks_cluster_load_balancer_dns;//fargate_construct.fargate_alb.loadBalancer.loadBalancerDnsName;
    this.vpc = props.vpc;
    this.api_ref = api_stack.api_ref;

    // new CfnOutput(this, "fargate_alb", {
    //   value: fargate_construct.fargate_alb.loadBalancer.loadBalancerDnsName,
    // });
  }
}
