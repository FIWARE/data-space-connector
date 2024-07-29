import { Aws } from "aws-cdk-lib"
import { InstanceClass, InstanceSize, InstanceType } from "aws-cdk-lib/aws-ec2"
import {StorageType } from "aws-cdk-lib/aws-rds"
import { Broker } from "./lib/stacks/garnet-constructs/constants"

export const Parameters = {
    // FIWARE DATA SPACE CONNECTOR PARAMETERS
    amazon_eks_cluster_load_balancer_dns: "k8s-kubesyst-ingressn-XXXXXXXX.elb.eu-west-1.amazonaws.com", // REPLACE WITH YOUR CLUSTER LOAD BALANCER DNS
    amazon_eks_cluster_load_balancer_listener_arn: "arn:aws:elasticloadbalancing:eu-west-1:XXXXXXXXX:listener/net/k8s-kubesyst-ingressn-XXXXXXXXXX/XXXXXXXXXXXXXXXX/XXXXXXXXXXXXXXXX",
    
    // GARNET PARAMETERS
    aws_region: "eu-west-1", // see regions in which you can deploy Garnet: https://docs.aws.amazon.com/apigateway/latest/developerguide/http-api-vpc-links.html#http-api-vpc-link-availability
    garnet_broker: Broker.SCORPIO, // DO NOT CHANGE 
    garnet_bucket: `garnet-datalake-${Aws.REGION}-${Aws.ACCOUNT_ID}`, // DO NOT CHANGE
    smart_data_model_url : 'https://raw.githubusercontent.com/awslabs/garnet-framework/main/context.jsonld',  
    // FARGATE PARAMETERS
    garnet_fargate: {
        fargate_cpu: 1024,
        fargate_memory_limit: 4096,
        autoscale_requests_number: 200, 
        autoscale_min_capacity: 2, 
        autoscale_max_capacity: 10
    },
    // SCORPIO BROKER PARAMETERS
    garnet_scorpio: {
        image_context_broker: 'public.ecr.aws/garnet/scorpio:4.1.10', // Link to ECR Public gallery of Scorpio Broker image.
        rds_instance_type: InstanceType.of( InstanceClass.BURSTABLE4_GRAVITON, InstanceSize.MEDIUM), // see https://aws.amazon.com/rds/instance-types/
        rds_storage_type: StorageType.GP3, // see https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/CHAP_Storage.html
        dbname: 'scorpio'
    }
}