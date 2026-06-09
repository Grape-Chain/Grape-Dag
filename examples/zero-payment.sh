#!/bin/bash

echo "Do Init "
for i in {1..5} 
do 
curl -k -X 'POST' \
   '${GRAPE_API_URL:-https://localhost:8010}/api/rest/transactions' \
   -H 'accept: application/json' \
   -H 'Authorization: Basic ${GRAPE_API_AUTH:?set GRAPE_API_AUTH (base64 user:pass)}'  \
   -H 'Content-Type: application/json' \
   -d '{
   "encodedTx": "0x2220940d33cca77608545439c121b679c23b6915f7990ff279cb902c2ccaff2057e32a14d09ec4a81cde61b57de012d3fe80beae3f28fb683214959cc0177d3cf38cb04104f40b1d66d05beb6edb4a0b08f4a0d2a00610fb87d1666a40281e6468fd4aad02549b19a4c96d15f996f887cad925c3bc09ae71b738eface593a2ff541e4003cb2c41b842b3f2fa8e21719d276283d9d3bf999af086b64d05"
}'
done