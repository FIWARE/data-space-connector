const { SQSClient, DeleteQueueCommand } = require("@aws-sdk/client-sqs")
const sqs = new SQSClient({})
const SQS_QUEUES = JSON.parse(process.env.SQS_QUEUES)

exports.handler = async (event) => {
    console.log(event)
    let request_type = event['RequestType'].toLowerCase()
    if (request_type=='delete') {

        try {
            for await (let queue of Object.values(SQS_QUEUES)){
                await sqs.send(
                    new DeleteQueueCommand({
                        QueueUrl: queue
                    })
                )
    
            }
            
        } catch (e) {
            console.log(e)
        }
        return true
    }
}