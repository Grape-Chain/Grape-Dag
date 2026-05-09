# LunaOne Protobuff Transactions

Protobuff v3 mappings for Luna Transaction. Mostly used for transfering transactions, their serialization and creation/signing.

## Overview 
This package is a GO package and contains GO Transaction code generated from Protobuff mappings **by default** and can be imported in any Golang application to create/sign/transfer Luna Transactions already

## Prerequisites
- Protobuff v3 compiler: 

    ```bash
    apt install -y protobuf-compiler
    protoc --version  # Ensure compiler version is 3+
    ```
- For Golang
  
    - Protobuff source generator plugins. 
        ```bash 
        go install google.golang.org/protobuf/cmd protoc-gen-go@v1.28
        go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2
        ```    


## Generating Transaction from Protobuff

- **For Golang** (this package provides by default)
  
  Run
  ```bash
  go generate
  ```
  See for `txvX.pb.go` file in current directory

- **For Java**
    
    Use [Maven Protobuff Plugin](https://www.xolstice.org/protobuf-maven-plugin/), see [example](https://www.xolstice.org/protobuf-maven-plugin/usage.html)

- **For JS**

    Run
    ```bash
    mkdir jspb
    protoc --proto_path=. --js_out=./jspb *.proto

    ```
    Then copy all the files from `jspb` directory to your project
- Other languages are also supported, see the [official site](https://developers.google.com/protocol-buffers/docs/reference/overview) for details


## Signing transaction

Transaction signing process is tightly coupled with transaction creation process and its serialization to ProtoBuff. 

The sample solution to create transaction, sign it, serialize to ProtoBuff, deserialize and verify its signature is given [here](createtx_test.go) in Golang. There we are using [luna1crypto](../luna1crypto/README.md) module (Golang) but there are other fully compatible crypto impls: [jscrypto](../luna1crypto/README.md) , [javacrypto](../javacrypto/README.md)

### General tx creation steps
1. Create empty Txv1 object (protobuff)
2. Populate it with your data: amount, your public key, nonce, data, recipient (base58 decoded first 21 bytes of address, no checksum), timestamp, etc
3. Leave a signature field empty
4. Serialize to protobuff using `proto.Marshal`
5. Sign serialized content using private key `luna1crypto.NewDSA().Sign(myPrivKey, unsignedBytes)`
6. Attach gotten signature to Txv1 object
7. Serialize it to Protobuff bytes again
8. Save the hash of transaction to check on its status. Hash of the transaction can calculated using signed tx protobuff bytes hashed via `SHA-256`
9. Send the serialized bytes encoded in hexadecimal format on endpoint POST `/transactions` where body would be `{"encodedTx": "0x123fac12302382"}` 
