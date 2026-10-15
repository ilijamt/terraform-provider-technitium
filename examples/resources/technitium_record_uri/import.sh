# the URI contains "/", so use the JSON form of the identity attributes
terraform import technitium_record_uri.sip '{"zone":"uri.example.com","domain":"_sip._tcp.uri.example.com","uri_priority":10,"uri_weight":1,"uri":"https://sip.uri.example.com/"}'
