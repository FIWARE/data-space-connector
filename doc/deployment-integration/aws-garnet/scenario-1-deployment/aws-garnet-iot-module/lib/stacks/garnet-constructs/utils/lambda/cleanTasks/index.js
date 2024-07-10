const { ECSClient, ListTaskDefinitionsCommand, DeleteTaskDefinitionsCommand } = require("@aws-sdk/client-ecs")
const ecs = new ECSClient({})


exports.handler = async (event) => {
    console.log(event)
    let request_type = event['RequestType'].toLowerCase()
    if (request_type=='delete') {

        try {

            const {taskDefinitionArns} = await ecs.send(
                new ListTaskDefinitionsCommand({
                    status: "INACTIVE"
                })
            )
        const inactive_garnet_tasks =  chunk(taskDefinitionArns.filter((task) => task.includes('Garnet')), 10)    
        for await (let task of inactive_garnet_tasks) {
                await ecs.send(
                    new DeleteTaskDefinitionsCommand({
                        taskDefinitions: task
                    })
                )

        }            
        } catch (e) {
            console.log(e)
        }
        return true
    }
}

const chunk = (arr, size) => Array.from({ length: Math.ceil(arr.length / size) }, (v, i) => arr.slice(i * size, i * size + size))
