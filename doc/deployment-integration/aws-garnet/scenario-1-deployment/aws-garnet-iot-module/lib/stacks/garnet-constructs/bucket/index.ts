import { CustomResource, Duration, Names } from "aws-cdk-lib";
import { Runtime, Function, Code, Architecture } from "aws-cdk-lib/aws-lambda";
import { Construct } from "constructs";
import { Parameters } from "../../../../parameters";
import { PolicyStatement, Role, ServicePrincipal } from "aws-cdk-lib/aws-iam";
import { Provider } from "aws-cdk-lib/custom-resources";
import { CfnDeliveryStream } from "aws-cdk-lib/aws-kinesisfirehose";
import { Bucket } from "aws-cdk-lib/aws-s3";

export interface GarnetBucketProps {

  }


export class GarnetBucket extends Construct {

    public readonly bucket_name: string
    public readonly kinesis_firehose: CfnDeliveryStream

    constructor(scope: Construct, id: string, props: GarnetBucketProps) {
        super(scope, id)

      // CUSTOM RESOURCE WITH A LAMBDA THAT WILL CREATE GARNET BUCKET AND ATHENA RESULTS BUCKET IF NOT EXISTS
      const lambda_bucket_path = `${__dirname}/lambda/bucketHead`
      const lambda_bucket = new Function(this, 'BucketHeadFunction', {
            functionName: `garnet-utils-bucket-lambda-${Names.uniqueId(this).slice(-4).toLowerCase()}`,
            description: 'Garnet Utils - Function that creates Garnet Bucket if it does not exist',
            runtime: Runtime.NODEJS_18_X,
            code: Code.fromAsset(lambda_bucket_path),
            handler: 'index.handler',
            timeout: Duration.seconds(50),
            architecture: Architecture.ARM_64,
            environment: {
              BUCKET_NAME: Parameters.garnet_bucket
            }
      })

      lambda_bucket.addToRolePolicy(new PolicyStatement({
          actions: ["s3:CreateBucket"],
          resources: ["arn:aws:s3:::*"] 
          }))

      const bucket_provider = new Provider(this, 'CustomBucketProvider', {
        onEventHandler: lambda_bucket,
        providerFunctionName:  `garnet-provider-custom-bucket-${Names.uniqueId(this).slice(-4).toLowerCase()}`,
      }) 

     const bucket_resource = new CustomResource(this, 'CustomBucketProviderResource', {
          serviceToken: bucket_provider.serviceToken,
          
      })

    this.bucket_name = Parameters.garnet_bucket 

    }
}