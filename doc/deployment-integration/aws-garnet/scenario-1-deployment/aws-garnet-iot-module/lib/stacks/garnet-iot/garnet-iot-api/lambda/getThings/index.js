const iot_region = process.env.AWSIOTREGION 
const shadow_prefix = process.env.SHADOW_PREFIX

const { IoTClient, SearchIndexCommand} = require("@aws-sdk/client-iot")
const iot = new IoTClient({region: iot_region})



exports.handler = async (event) => {

    try { 
        let { headers, queryStringParameters} = event

        let index = 'Thing'

        if(queryStringParameters && queryStringParameters['index']) {
            index = queryStringParameters['index']
        }

        // lower case headers
        headers = Object.keys(headers).reduce( (acc, key) => {
            acc[key.toLowerCase()] = headers[key]
            return acc
        }, {})
      
        let nToken = headers?.['nexttoken'] ? headers['nexttoken'] : null

        let {things, nextToken} = await iot.send(
            new SearchIndexCommand({
                    nextToken: nToken,
                    queryString: `thingName:*`,
                    indexName: 'AWS_Things'
            })
        )

        let tgs = things.reduce((prev, curr) => { 
            return [ ...prev, 
                    { thingName: curr.thingName, 
                      thingGroupNames: curr.thingGroupNames, 
                      thingTypeName: curr.thingTypeName
                    }
            ]
        }, [])

    
        const response = {
            statusCode: 200,
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({
                nextToken, 
                things: tgs
            }),
        }


        return response

    }  catch(e){

        if(e.statusCode == 400){
            let statusCode = e.statusCode ? e.statusCode : 500
            const response = {
                statusCode,
                headers: {
                    "Content-Type": "application/json"
                },
                body: JSON.stringify({message: `Error with the submitted index. Please verify it is a valid index and registered in the Garnet IoT Index`}),
            } 
        }

        let statusCode = e.statusCode ? e.statusCode : 500
        const response = {
            statusCode,
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({message: e.message}),
        }
        console.log(e)
        return response
    }
}