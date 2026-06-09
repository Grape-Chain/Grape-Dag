# GrapeOne Crypto operations (Java)

## Overview

This module is an official fully compatible implementation of signing, verifying and hashing operations for GrapeOne
project where the reference impl is [grape1crypto](../grape1crypto/README.md)

## Build

Project is using Java17 + Maven 3.8.x for a build&run process

Use `mvn clean install` to build the project

## Usage

This module can be imported to any Java17 maven project using a local maven repository or artifactory (not added yet):

```xml

<dependency>
    <groupId>io.aplfintech.grape.grap3</groupId>
    <artifactId>grap3-crypto</artifactId>
    <version>0.4</version>
</dependency>
```

When module is imported then you can use Sign/Verify/Hash operations like given
in [test](src/test/java/io/aplfintech/grape/grap3/CryptoTest.java)
All operations are using the same messages and keys to ensure that correctness and compatibility of algorithms is
preserved
