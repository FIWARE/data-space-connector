const VPC_ENDPOINT = process.env.VPC_ENDPOINT
const LAKERULENAME = process.env.LAKERULENAME

exports.handler = async (event) => {

    let auth = {}
    auth.isAuthenticated = true 
    auth.principalId = "garnetDatalakePrincipal"
    let policyDocument = {}
    policyDocument.Version = "2012-10-17"
    policyDocument.Statement = []
    let publishStatement = {}
    publishStatement.Action = ["iot:Publish"]
    publishStatement.Effect = "Allow"
    publishStatement.Resource = [`arn:aws:iot:*:*:topic/$aws/rules/${LAKERULENAME}`]
    publishStatement.Condition = {}
    publishStatement.Condition.StringEquals = {}
    publishStatement.Condition.StringEquals["aws:SourceVpce"] = VPC_ENDPOINT
    
    policyDocument.Statement[0] = publishStatement
    
    auth.policyDocuments = [policyDocument]
    auth.disconnectAfterInSeconds = 86400
    auth.refreshAfterInSeconds = 86400

    return auth 


}