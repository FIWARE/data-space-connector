const iot_region = process.env.AWSIOTREGION 
const sub_all = process.env.SUBALL
const { IoTDataPlaneClient, PublishCommand } = require("@aws-sdk/client-iot-data-plane")
const iotdata = new IoTDataPlaneClient({region: iot_region})

exports.handler = async (event) => {
    try {
        const {body} = event
        if(!body){
            return {
                statusCode: 400, 
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify({message: 'Bad Request. Notification is the only type valid'})
            }
        }
        const payload = JSON.parse(body)

        if(payload?.type != "Notification") {
            console.log('ERROR not Notification')
            return {
                statusCode: 400, 
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify({message: 'Bad Request. Notification is the only type valid'})
            }
        }
         // GET THE SUBSCRIPTION NAME FROM SUBSCRIPTION ID
         const subName = `${payload.subscriptionId.split(':').slice(-1)}`

         payload.data.forEach(entity => {
            for (let [key, value] of Object.entries(entity)) {
                if(!['type', 'id', '@context'].includes(key)) {
            
                    if( typeof value == 'object' && !Array.isArray(value)){
                    recursive_concise(key, value)
                    } else {
                        entity[key] = {
                            value: value
                        }
                    }
                    
                
                } 
            }
        })

        const publish = await iotdata.send(
            new PublishCommand({
                topic: `garnet/subscriptions/${subName}`,
                payload: JSON.stringify(payload)
            })
        )


         const response = {
            statusCode: 200
        }
        return response

    } catch (e) {
        const response = {
            statusCode: 500,
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({message: e.message}),
        }
        console.log(e)
        return response

    }


}

const recursive_concise = (key, value) => {

    if( typeof value == 'object' && !Array.isArray(value)){
        if(['Property', 'Relationship', 'GeoProperty'].includes(value['type'])) {
            delete value['type'] 
        }
        for (let [skey, svalue] of Object.entries(value)){
                    recursive_concise(skey,svalue)
        }
    }
}