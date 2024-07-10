const { AthenaClient, CreateWorkGroupCommand } = require("@aws-sdk/client-athena")
const { GlueClient, CreateDatabaseCommand } = require("@aws-sdk/client-glue")
const athena = new AthenaClient()
const glue = new GlueClient()
const BUCKET_NAME = process.env.BUCKET_NAME
const CATALOG_ID = process.env.CATALOG_ID
const GLUEDB_NAME = process.env.GLUEDB_NAME

exports.handler = async (event) => {
    console.log(event)
    let request_type = event['RequestType'].toLowerCase()
    if (request_type=='create' || request_type == 'update') {

        try {
            await athena.send(
                new CreateWorkGroupCommand({
                    Name: 'garnet',
                    Configuration: {
                        ResultConfiguration: { 
                            OutputLocation: `s3://${BUCKET_NAME}-athena-results/`
                        }
                    }
                })
            )        
        } catch (e) {
            console.log(e.message)
        }

        try {
            await glue.send(
                new CreateDatabaseCommand({
                    CatalogId: CATALOG_ID, 
                    DatabaseInput: {
                        Name: GLUEDB_NAME
                    }
                })
            )        
        } catch (e) {
            console.log(e.message)
        }

        return true
    }
}