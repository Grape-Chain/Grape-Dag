# Luna One Node API

---
Type: REST

Protocol: HTTPS

Format: JSON


Luna One node's API is defined as an OpenAPI v3 specification located in `swagger/openapi.yml` file

## Setting up the client
To get the client boilerplate auto-generated code from this spec, you can use `Swagger Editor`
```schell
docker pull swaggerapi/swagger-editor
docker run -d -p 80:8080 swaggerapi/swagger-editor
```
Then open your browser and type url: `localhost` after that import the `api/openapi.yml` file

Use `Generate Client` section to get the needed type of client: JavaScript, Angular, Typescript, Java, Go are supported and many more

## Development

To generate the boilerplate Go server from openapi.yaml you just need to run:
```shell
go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@latest
oapi-codegen --config api/oapi-config.yml api/openapi.yml
```
Edit `oapi-config.yml` file to generate a different server code or include other OpenApi v3 specs

### TLS Setup
1. Go to ${HOME_DIR}/.lunaone
2. Generate self-signed mock TLS key + cert: `openssl req -new -newkey rsa:4096 -x509 -sha256 -days 365 -nodes -out luna-tls.crt -keyout luna-tls.key`
3. Add `peer.apikey=luna-tls.key` and `peer.apicert=luna-tls.crt` to lunapeer.yml
4. Start the node: `go run . -id 1`
5. Test (export `LUNA_REST_API_USERNAME` and `LUNA_REST_API_PASSWORD` first):
   `curl -k "${LUNA_API_URL:-https://localhost:8010}/api/rest/peers" -H 'accept: application/json' -H "Authorization: Basic $(printf '%s' "$LUNA_REST_API_USERNAME:$LUNA_REST_API_PASSWORD" | base64)"`
