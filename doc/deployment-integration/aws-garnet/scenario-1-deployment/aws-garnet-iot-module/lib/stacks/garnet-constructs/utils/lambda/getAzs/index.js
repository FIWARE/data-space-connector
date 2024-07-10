const { EC2Client, DescribeAvailabilityZonesCommand, DescribeVpcEndpointServicesCommand } = require("@aws-sdk/client-ec2")
const ec2 = new EC2Client({apiVersion: '2016-11-15'})
const compatible_azs = JSON.parse(process.env.COMPATIBLE_AZS)
let region = process.env.AWS_REGION

exports.handler = async (event) => {
    console.log(event)
    let request_type = event['RequestType'].toLowerCase()
    if (request_type=='create' || request_type == 'update') {


        const {AvailabilityZones} = await ec2.send(
            new DescribeAvailabilityZonesCommand({})
        )

        const {ServiceDetails} = await ec2.send(
            new DescribeVpcEndpointServicesCommand({
                ServiceNames: [`com.amazonaws.${region}.iot.data`]
            })
        )

        let vpc_link_az = AvailabilityZones.filter((az) => compatible_azs.includes(az.ZoneId)).map((az) => az.ZoneName)
        let vpc_endpoint_iot_az = ServiceDetails[0]["AvailabilityZones"]

        let final_azs = vpc_link_az.filter((arr) => vpc_endpoint_iot_az.indexOf(arr) !== -1)
        console.log({final_azs})
        
        return {
            Data: {
                az1: final_azs[0],
                az2: final_azs[final_azs.length - 1]
            }
        }
    }
}