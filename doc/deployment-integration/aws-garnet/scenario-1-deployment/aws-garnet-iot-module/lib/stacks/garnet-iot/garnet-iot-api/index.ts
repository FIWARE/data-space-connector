import { Aws, Duration, Names } from "aws-cdk-lib"

import { CfnIntegration, CfnRoute, CfnVpcLink } from "aws-cdk-lib/aws-apigatewayv2"
import { SubnetType, Vpc } from "aws-cdk-lib/aws-ec2"
import { PolicyStatement } from "aws-cdk-lib/aws-iam"
import { Runtime, Function, Code, CfnPermission, LayerVersion, Architecture } from "aws-cdk-lib/aws-lambda"
import { RetentionDays } from "aws-cdk-lib/aws-logs"
import { AwsCustomResource, AwsCustomResourcePolicy, PhysicalResourceId } from "aws-cdk-lib/custom-resources"
import { Construct } from "constructs"
import { Parameters } from "../../../../parameters"
import { garnet_constant } from "../../garnet-constructs/constants"


export interface GarnetIotApiProps {
    readonly api_ref: string,
    readonly vpc: Vpc,
    dns_context_broker: string
}

export class GarnetIotApi extends Construct {
   
    constructor(scope: Construct, id: string, props: GarnetIotApiProps){
        super(scope, id)


        // LAMBDA LAYER (SHARED LIBRARIES)
        const layer_lambda_path = `./lib/layers`
        const layer_lambda = new LayerVersion(this, 'LayerLambda', {
            code: Code.fromAsset(layer_lambda_path),
            compatibleRuntimes: [Runtime.NODEJS_18_X]
        })


// ********************************************** 

        /**
         *  GARNET VERSION 
         */

        // LAMBDA GARNET API VERSION
        const lambda_garnet_version_path = `${__dirname}/lambda/garnetVersion`
        const lambda_garnet_version = new Function(this, 'LambdaGarnetVersion', {
            functionName: `garnet-api-version-lambda-${Names.uniqueId(this).slice(-4).toLowerCase()}`,
            description: 'Garnet API - Function that returns the Garnet Version',
            runtime: Runtime.NODEJS_18_X,
            code: Code.fromAsset(lambda_garnet_version_path),
            handler: 'index.handler',
            timeout: Duration.seconds(30),
            logRetention: RetentionDays.THREE_MONTHS,
            layers: [layer_lambda],
            architecture: Architecture.ARM_64,

            environment: {
                CONTEXT_BROKER: Parameters.garnet_broker,
                GARNET_VERSION: garnet_constant.garnet_version
                }   
        })

        const garnet_version_integration = new CfnIntegration(this, 'GarnetVersionIntegration', {
            apiId: props.api_ref,
            integrationMethod: "GET",
            integrationType: "AWS_PROXY",
            integrationUri: lambda_garnet_version.functionArn,
            connectionType: "INTERNET",
            description: "GARNET VERSION INTEGRATION",
            payloadFormatVersion: "1.0",
        })

        const garnet_version_route = new CfnRoute(this, 'GarnetVersionRoute', {
            apiId: props.api_ref,
            routeKey: "GET /",
            target: `integrations/${garnet_version_integration.ref}`
        })

        new CfnPermission(this, 'ApiGatewayLambdaPermissionGarnetVersion', {
            principal: `apigateway.amazonaws.com`,
            action: 'lambda:InvokeFunction',
            functionName: lambda_garnet_version.functionName,
            sourceArn: `arn:aws:execute-api:${Aws.REGION}:${Aws.ACCOUNT_ID}:${props.api_ref}/*/*/*`
        })

        /**
        *  END GARNET VERSION
        */


// ********************************************** 



// ********************************************** 

        /**
         *  POST THING
         */

        // LAMBDA THAT POSTS THING
        const lambda_post_thing_path = `${__dirname}/lambda/postThing`
        const lambda_post_thing = new Function(this, 'LambdaPostThing', {
            functionName: `garnet-iot-api-post-thing-lambda-${Names.uniqueId(this).slice(-4).toLowerCase()}`,
            description: 'Garnet API - Function to POST THING',
            runtime: Runtime.NODEJS_18_X,
            code: Code.fromAsset(lambda_post_thing_path),
            handler: 'index.handler',
            timeout: Duration.seconds(30),
            logRetention: RetentionDays.THREE_MONTHS,
            layers: [layer_lambda],
            architecture: Architecture.ARM_64,
            environment: {
                AWSIOTREGION: Aws.REGION,
                SHADOW_PREFIX: garnet_constant.shadow_prefix,
                }   
        })

        lambda_post_thing.addToRolePolicy(new PolicyStatement({
            actions: [
                "iot:UpdateThingGroup",
                "iot:CreateThingGroup",
                "iot:CreateThing",
                "iot:AddThingToThingGroup",
                "iot:UpdateThingGroupsForThing",
                "iot:UpdateThingShadow"
            ],
            resources: [
                `arn:aws:iot:${Aws.REGION}:${Aws.ACCOUNT_ID}:thinggroup/*`,
                `arn:aws:iot:${Aws.REGION}:${Aws.ACCOUNT_ID}:thing/*`
            ]
        }))

        const post_thing_integration = new CfnIntegration(this, 'postThingIntegration', {
            apiId: props.api_ref,
            integrationMethod: "POST",
            integrationType: "AWS_PROXY",
            integrationUri: lambda_post_thing.functionArn,
            connectionType: "INTERNET",
            description: "POST THING INTEGRATION",
            payloadFormatVersion: "1.0",
        })

        const post_thing_route = new CfnRoute(this, 'PostThingRoute', {
            apiId: props.api_ref,
            routeKey: "POST /iot/things",
            target: `integrations/${post_thing_integration.ref}`
        })

        new CfnPermission(this, 'ApiGatewayLambdaPermissionPostThing', {
            principal: `apigateway.amazonaws.com`,
            action: 'lambda:InvokeFunction',
            functionName: lambda_post_thing.functionName,
            sourceArn: `arn:aws:execute-api:${Aws.REGION}:${Aws.ACCOUNT_ID}:${props.api_ref}/*/*/*`
        })

        /**
        *  END POST THING 
        */


// ********************************************** 

        /**
        *  DELETE THING 
        */

        // LAMBDA THAT DELETE THING
        const lambda_delete_thing_path = `${__dirname}/lambda/deleteThing`
        const lambda_delete_thing = new Function(this, 'LambdaDeleteThing', {
            functionName: `garnet-iot-api-delete-thing-lambda-${Names.uniqueId(this).slice(-4).toLowerCase()}`,
            description: 'Garnet API - Function to DELETE THING',
            vpc: props.vpc, 
            vpcSubnets: {
                subnetType: SubnetType.PRIVATE_WITH_EGRESS
            },
            runtime: Runtime.NODEJS_18_X,
            code: Code.fromAsset(lambda_delete_thing_path),
            handler: 'index.handler',
            timeout: Duration.seconds(30),
            logRetention: RetentionDays.THREE_MONTHS,
            layers: [layer_lambda],
            architecture: Architecture.ARM_64,
            environment: {
                AWSIOTREGION: Aws.REGION,
                SHADOW_PREFIX: garnet_constant.shadow_prefix,
                DNS_CONTEXT_BROKER: props.dns_context_broker
            }   
        })

        lambda_delete_thing.addToRolePolicy(new PolicyStatement({
            actions: [
                "iot:DeleteThing",
                "iot:DeleteThingShadow",
                "iot:ListNamedShadowsForThing",

            ],
            resources: [
                `arn:aws:iot:${Aws.REGION}:${Aws.ACCOUNT_ID}:thing/*`
            ]
        }))

        const delete_thing_integration = new CfnIntegration(this, 'deleteThingIntegration', {
            apiId: props.api_ref,
            integrationMethod: "DELETE",
            integrationType: "AWS_PROXY",
            integrationUri: lambda_delete_thing.functionArn,
            connectionType: "INTERNET",
            description: "DELETE THING INTEGRATION",
            payloadFormatVersion: "1.0",
        })

        const delete_thing_route = new CfnRoute(this, 'DeleteThingRoute', {
            apiId: props.api_ref,
            routeKey: "DELETE /iot/things/{thingName}",
            target: `integrations/${delete_thing_integration.ref}`
        })

        new CfnPermission(this, 'ApiGatewayLambdaPermissionDeleteThing', {
            principal: `apigateway.amazonaws.com`,
            action: 'lambda:InvokeFunction',
            functionName: lambda_delete_thing.functionName,
            sourceArn: `arn:aws:execute-api:${Aws.REGION}:${Aws.ACCOUNT_ID}:${props.api_ref}/*/*/*`
        })

    /***
     * END DELETE THING 
     */

/************************************************************************** */


        /**
         *  GET THING
         */

        // LAMBDA THAT GETS THING
        const lambda_get_thing_path = `${__dirname}/lambda/getThing`
        const lambda_get_thing = new Function(this, 'LambdaGetThing', {
            functionName: `garnet-iot-api-get-thing-lambda-${Names.uniqueId(this).slice(-4).toLowerCase()}`,
            description: 'Garnet API - Function to GET THING',
            runtime: Runtime.NODEJS_18_X,
            code: Code.fromAsset(lambda_get_thing_path),
            handler: 'index.handler',
            timeout: Duration.seconds(30),
            logRetention: RetentionDays.THREE_MONTHS,
            layers: [layer_lambda],
            architecture: Architecture.ARM_64,
            environment: {
                AWSIOTREGION: Aws.REGION,
                SHADOW_PREFIX: garnet_constant.shadow_prefix,
                }   
        })

        lambda_get_thing.addToRolePolicy(new PolicyStatement({
            actions: [
                "iot:GetThing",
                "iot:listNamedShadowsForThing",
                "iot:getThingShadow"
            ],
            resources: [
                `arn:aws:iot:${Aws.REGION}:${Aws.ACCOUNT_ID}:thing/*`
            ]
        }))

        const get_thing_integration = new CfnIntegration(this, 'getThingIntegration', {
            apiId: props.api_ref,
            integrationMethod: "GET",
            integrationType: "AWS_PROXY",
            integrationUri: lambda_get_thing.functionArn,
            connectionType: "INTERNET",
            description: "GET THING INTEGRATION",
            payloadFormatVersion: "1.0",
        })

        const get_thing_route = new CfnRoute(this, 'GetThingRoute', {
            apiId: props.api_ref,
            routeKey: "GET /iot/things/{thingName}",
            target: `integrations/${get_thing_integration.ref}`
        })

        new CfnPermission(this, 'ApiGatewayLambdaPermissionGetThing', {
            principal: `apigateway.amazonaws.com`,
            action: 'lambda:InvokeFunction',
            functionName: lambda_get_thing.functionName,
            sourceArn: `arn:aws:execute-api:${Aws.REGION}:${Aws.ACCOUNT_ID}:${props.api_ref}/*/*/*`
        })

        /**
        *  END GET THING 
        */

        /************************************************************************** */


        /**
         *  GET THINGS
         */

        // LAMBDA THAT GETS THING
        const lambda_get_things_path = `${__dirname}/lambda/getThings`
        const lambda_get_things = new Function(this, 'LambdaGetThings', {
            functionName: `garnet-iot-api-get-things-lambda-${Names.uniqueId(this).slice(-4).toLowerCase()}`,
            description: 'Garnet API - Function to GET THINGS',
            runtime: Runtime.NODEJS_18_X,
            code: Code.fromAsset(lambda_get_things_path),
            handler: 'index.handler',
            timeout: Duration.minutes(3),
            logRetention: RetentionDays.THREE_MONTHS,
            layers: [layer_lambda],
            architecture: Architecture.ARM_64,
            environment: {
                AWSIOTREGION: Aws.REGION,
                SHADOW_PREFIX: garnet_constant.shadow_prefix,
                }   
        })

        lambda_get_things.addToRolePolicy(new PolicyStatement({
            actions: [
                "iot:searchIndex"
            ],
            resources: [
                `arn:aws:iot:${Aws.REGION}:${Aws.ACCOUNT_ID}:index/*`
            ]
        }))

        const get_things_integration = new CfnIntegration(this, 'getThingsIntegration', {
            apiId: props.api_ref,
            integrationMethod: "GET",
            integrationType: "AWS_PROXY",
            integrationUri: lambda_get_things.functionArn,
            connectionType: "INTERNET",
            description: "GET THINGS INTEGRATION",
            payloadFormatVersion: "1.0",
        })

        const get_things_route = new CfnRoute(this, 'GetThingsRoute', {
            apiId: props.api_ref,
            routeKey: "GET /iot/things",
            target: `integrations/${get_things_integration.ref}`
        })

        new CfnPermission(this, 'ApiGatewayLambdaPermissionGetThings', {
            principal: `apigateway.amazonaws.com`,
            action: 'lambda:InvokeFunction',
            functionName: lambda_get_things.functionName,
            sourceArn: `arn:aws:execute-api:${Aws.REGION}:${Aws.ACCOUNT_ID}:${props.api_ref}/*/*/*`
        })

        /**
        *  END GET THINGS
        */



        /************************************************************************** */


        /**
         *  POST SHADOWS
         */

        // LAMBDA THAT POST SHADOWS
        const lambda_post_shadows_path = `${__dirname}/lambda/postShadows`
        const lambda_post_shadows = new Function(this, 'LambdaPostShadows', {
            functionName: `garnet-iot-api-post-shadows-lambda-${Names.uniqueId(this).slice(-4).toLowerCase()}`,
            description: 'Garnet API - Function to POST SHADOW',
            runtime: Runtime.NODEJS_18_X,
            code: Code.fromAsset(lambda_post_shadows_path),
            handler: 'index.handler',
            timeout: Duration.seconds(50),
            logRetention: RetentionDays.THREE_MONTHS,
            layers: [layer_lambda],
            architecture: Architecture.ARM_64,
            environment: {
                AWSIOTREGION: Aws.REGION,
                SHADOW_PREFIX: garnet_constant.shadow_prefix,
                }   
        })

        lambda_post_shadows.addToRolePolicy(new PolicyStatement({
            actions: [
                "iot:UpdateThingShadow"
            ],
            resources: [
                `arn:aws:iot:${Aws.REGION}:${Aws.ACCOUNT_ID}:thing/*`
            ]
        }))

        const post_shadows_integration = new CfnIntegration(this, 'postShadowsIntegration', {
            apiId: props.api_ref,
            integrationMethod: "POST",
            integrationType: "AWS_PROXY",
            integrationUri: lambda_post_shadows.functionArn,
            connectionType: "INTERNET",
            description: "GET THING INTEGRATION",
            payloadFormatVersion: "1.0",
        })

        const post_shadows_route = new CfnRoute(this, 'PostShadowsRoute', {
            apiId: props.api_ref,
            routeKey: "POST /iot/things/{thingName}/shadows",
            target: `integrations/${post_shadows_integration.ref}`
        })

        new CfnPermission(this, 'ApiGatewayLambdaPermissionPostShadows', {
            principal: `apigateway.amazonaws.com`,
            action: 'lambda:InvokeFunction',
            functionName: lambda_post_shadows.functionName,
            sourceArn: `arn:aws:execute-api:${Aws.REGION}:${Aws.ACCOUNT_ID}:${props.api_ref}/*/*/*`
        })

        /**
        *  END POST SHADOWS
        */







    }
}