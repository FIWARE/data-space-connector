import { NestedStack, NestedStackProps } from 'aws-cdk-lib'
import { Vpc } from 'aws-cdk-lib/aws-ec2'
import { Construct } from 'constructs'
import { GarnetIotApi } from './garnet-iot-api'
import { GarnetIot } from './garnet-iot-ingestion'
import { GarnetPrivateSub } from './garnet-iot-private-sub'
import { GarnetLake } from './garnet-iot-lake'

export interface GarnetIotStackProps extends NestedStackProps {
  dns_context_broker: string,
  vpc: Vpc, 
  api_ref: string,
  bucket_name: string,
  az1: string,
  az2: string
}

export class GarnetIotStack extends NestedStack {

  public readonly iot_sqs_endpoint_url : string
  public readonly private_sub_endpoint: string

  constructor(scope: Construct, id: string, props: GarnetIotStackProps) {
    super(scope, id, props)

    // GARNET IoT CORE
    const garnet_iot_core_construct = new GarnetIot(this, 'Core', {
      vpc: props.vpc, 
      dns_context_broker: props.dns_context_broker
    })

    // GARNET IOT API 
    const garnet_iot_api_construct= new GarnetIotApi(this, 'Api', {
      api_ref: props.api_ref,
      vpc: props.vpc, 
      dns_context_broker: props.dns_context_broker
    })

    // GARNET IOT PRIVATE SUB 
    const garnet_private_sub_construct = new GarnetPrivateSub(this, "PrivateSub", {
      vpc: props.vpc,
      bucket_name: props.bucket_name
    })


    // GARNET IOT DATA LAKE 
    const lake_construct = new GarnetLake(this, 'Lake', {
      vpc: props.vpc, 
      bucket_name: props.bucket_name,
      dns_context_broker: props.dns_context_broker,
      az1: props.az1,
      az2: props.az2
    })


    this.iot_sqs_endpoint_url = garnet_iot_core_construct.sqs_garnet_iot_url
    this.private_sub_endpoint = garnet_private_sub_construct.private_sub_endpoint
  }
}
