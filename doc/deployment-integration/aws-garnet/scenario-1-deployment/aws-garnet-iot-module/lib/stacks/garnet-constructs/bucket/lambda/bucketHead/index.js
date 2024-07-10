const { S3Client, CreateBucketCommand } = require("@aws-sdk/client-s3")
const s3 = new S3Client()
const BUCKET_NAME = process.env.BUCKET_NAME

exports.handler = async (event) => {
    console.log(event)
    let request_type = event['RequestType'].toLowerCase()
    if (request_type=='create' || request_type == 'update') {

        try {
            await s3.send(
                new CreateBucketCommand({
                    Bucket: BUCKET_NAME
                })
            )
        } catch (e) {
            console.log(e.message)
        }

        try {
            console.log("UPDATE HERE")
            await s3.send(
                new CreateBucketCommand({
                    Bucket: `${BUCKET_NAME}-athena-results`
                })
            )
        } catch (e) {
            console.log(e.message)
        }

        return true
    }
}