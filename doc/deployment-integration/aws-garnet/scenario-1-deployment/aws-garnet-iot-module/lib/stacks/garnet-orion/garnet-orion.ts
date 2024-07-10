import { Aws, CfnOutput, NestedStack, NestedStackProps } from "aws-cdk-lib"
import { Construct } from "constructs"
import { GarnetSecret } from "../garnet-constructs/secret";
import { GarnetNetworking } from "../garnet-constructs/networking";
//import { GarnetOrionDatabase } from "./database";
//import { GarnetOrionFargate } from "./fargate";
import { Parameters } from "../../../parameters";
import { Vpc } from "aws-cdk-lib/aws-ec2";
import { GarnetApiGateway } from "../garnet-constructs/apigateway";
import { Secret } from "aws-cdk-lib/aws-secretsmanager";

export interface GarnetOrionProps extends NestedStackProps{
  vpc: Vpc,
  secret: Secret
}

export class GarnetOrion extends NestedStack {

  public readonly dns_context_broker: string
  public readonly broker_api_endpoint: string
  public readonly api_ref: string
  

  constructor(scope: Construct, id: string, props: GarnetOrionProps) {
    super(scope, id, props)

    const api_stack = new GarnetApiGateway(this, "Api", {
      vpc: props.vpc,
      fargate_albListenerArn: Parameters.amazon_eks_cluster_load_balancer_listener_arn, //TODO Replace with EKS Cluster Load Balancer Listener ARN // Pass the listenerArn as a string fargate_alb: ``//fargate_construct.fargate_alb,
    })

    new CfnOutput(this, "garnet_endpoint", {
      value: `https://${api_stack.api_ref}.execute-api.${Aws.REGION}.amazonaws.com`,
    })

    this.broker_api_endpoint = `https://${api_stack.api_ref}.execute-api.${Aws.REGION}.amazonaws.com`;
    this.dns_context_broker = Parameters.amazon_eks_cluster_load_balancer_dns //TODO Replace with EKS Cluster Load Balancer DNS name //fargate_construct.fargate_alb.loadBalancer.loadBalancerDnsName;
    this.api_ref = api_stack.api_ref;

  }

}
