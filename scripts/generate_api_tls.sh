#!/bin/bash

# This script generates a private key and a certificate to be used
# in grapepeer rest api with tls


# This functions prints the usage information
function print_usage() {
	echo "*******************************************************"
	echo "* Usage:                                              *"
	echo "*      $0 <path> <name>"
   echo "*      where:                                         *"
   echo "*           <path> - a directory where to write the   *"
   echo "*                    files to                         *"
   echo "*           <name> - the name of the key and certif.  *"
   echo "* The resulting artifacts will be written to the loc. *"
   echo "* <path>: <name>.key, <name>.csr, <name>.crt          *"
	echo "*******************************************************"
}

# This function generates a private key and a certificate request
function generate_key_csr() {
	openssl req -new -newkey rsa:2048 -nodes -keyout "$1".key -out "$1".csr
}

# This function generates a certificate and signs it with the private key
function generate_certificate() {
	openssl x509 -req -days 365 -in "$1".csr -signkey "$1".key -out "$1".crt
}

echo "Generate a self-signed certificate for the use by Grape 1 API TLS"
if [ "$#" -lt 2 ]; then
	echo "<Error> This script expects exactly two parameters"
	print_usage $0
	exit 0
fi

# Remember current directory - will need to return to it when done
CUR_DIR="$( pwd )"

# Turn off error handling for path checking
set +e
TESTPATH="$( cd ${1} && pwd )"
set -e

#Check if the path exists
if [[ "$TESTPATH" == "" ]]; then
	echo -e "The path $TESTPATH does not exist. Please provide a valid path"
	exit 0
fi

# Change dir to where the key and cert are to be generated
cd ${TESTPATH}

echo "STEP 1 - generate $2.key private key and $2.csr certificate request"
generate_key_csr $2 
if (( $? != 0 )); then
	echo -e "STEP 1 failed"
	exit 0
fi

echo "STEP 2 - generate $2.crt self signed certificate"
generate_certificate $2 
if (( $? != 0 )); then
	echo -e "STEP 2 failed"
	exit 0
fi

# Change back to the prev loc
cd ${CUR_DIR}

# We are done
echo "Successfully generated $1$2.crt certificate"

#EOF
