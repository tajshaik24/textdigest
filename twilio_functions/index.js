'use strict'

const AWS = require('aws-sdk')
AWS.config.update({
  accessKeyId: process.env.AWS_KEY,
  secretAccessKey: process.env.AWS_SECRET
})


function formatDate() {
  var today = new Date(),
      month = '' + (today.getMonth() + 1),
      day = '' + today.getDate(),
      year = today.getFullYear();

  if (month.length < 2) month = '0' + month;
  if (day.length < 2) day = '0' + day;

  return [month, day, year].join('-');
}

exports.handler = function(context, event, callback) {
    var dynamodb = new AWS.DynamoDB({ region: 'us-west-1' });
    let tdy_date = formatDate();
    var title = "";
    var params = {
      ExpressionAttributeValues: {
       ":v1": {
         S: tdy_date
        }
      }, 
      KeyConditionExpression: "#dt = :v1",
      TableName: "TextDigest",
      IndexName: "Date-index",
      ExpressionAttributeNames: {
        '#dt' : 'Date'
       }, 
     };
     
     dynamodb.query(params, (err, data) => {
       if (err){
        console.log(err); 
       } 
       else{
        let twiml = new Twilio.twiml.MessagingResponse();
        for (var i = 0; i < data["Items"].length; i++) { 
            var title = data["Items"][i]["Title"]['S'];
            var link = data["Items"][i]["Link"]['S'];
            var summary = data["Items"][i]["Summary"]['L'][0]['S'] + data["Items"][i]["Summary"]['L'][1]['S'];
            twiml.message(title + " " + link);
            twiml.message(summary);
        }
        callback(null, twiml);
       }        
     });
};