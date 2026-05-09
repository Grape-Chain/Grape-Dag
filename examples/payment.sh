#!/bin/bash

echo "Do Payment"

curl -k -X 'POST' \
   '${LUNA_API_URL:-https://localhost:8010}/api/rest/transactions' \
   -H 'accept: application/json' \
   -H 'Authorization: Basic ${LUNA_API_AUTH:?set LUNA_API_AUTH (base64 user:pass)}'  \
   -H 'Content-Type: application/json' \
   -d '{
   "encodedTx": "0x2220940d33cca77608545439c121b679c23b6915f7990ff279cb902c2ccaff2057e32a14d09ec4a81cde61b57de012d3fe80beae3f28fb683214959cc0177d3cf38cb04104f40b1d66d05beb6edb3a0a152d02c7e14af680000040024a0c08aa94dd9f061092f9acd201520224c25a010b6a40e041a3423a607ed6edbcea82266c73c16c7d0a6676a7c8463f63964ca3bf8b7fb34e8b141382cb9aa9488908456d4f5cb76b3b1c63fe39b57e6f32155c7d7f07"
}'