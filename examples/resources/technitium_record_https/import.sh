# svc_params is a map, so use the JSON form of the identity attributes
terraform import technitium_record_https.web '{"zone":"https.example.com","domain":"https.example.com","svc_priority":1,"svc_target_name":".","svc_params":{"alpn":"h2,h3","port":"443"}}'
