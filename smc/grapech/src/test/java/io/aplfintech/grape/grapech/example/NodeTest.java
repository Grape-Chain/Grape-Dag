package io.aplfintech.grape.grapech.example;

import com.fasterxml.jackson.annotation.JsonProperty;
import io.aplfintech.grape.grapech.Grapech;
import io.aplfintech.grape.grapech.TxBuilder;
import io.aplfintech.grape.utils.HexUtils;
import io.restassured.RestAssured;
import io.restassured.http.ContentType;
import io.restassured.parsing.Parser;
import io.restassured.response.Response;
import lombok.extern.slf4j.Slf4j;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Disabled;
import org.junit.jupiter.api.Test;

import java.util.Map;

import static io.restassured.RestAssured.given;
import static org.hamcrest.Matchers.is;

/**
 * @author andrew.zinchenko@gmail.com
 * @since 0.1
 */
@Slf4j
class NodeTest {
    static String genesisAddrHex = "0xd09ec4a81cde61b57de012d3fe80beae3f28fb68";
    static String genesisPubKeyHex = "940d33cca77608545439c121b679c23b6915f7990ff279cb902c2ccaff2057e3";
    static String genesisPrivKeyHex = "6f1c1e3f54a6699be61d927f804a191b90912820d89d8d5a8b143e1990fcc0af";

    static String baseUri = "https://localhost:8010/api/rest";
    static String authHeader = "Basic bHVuYWRldjpiSFZ1WVdSbGRnbz0=";
    static String transactionsEndpoint = "/transactions";
    static String estimateEndpoint = "/transactions/estimate";
    static String accountsEndpoint = "/accounts/{address}";

    @BeforeAll
    static void beforeAll() {
        RestAssured.useRelaxedHTTPSValidation();
        RestAssured.registerParser("text/plain", Parser.JSON);
    }

    @Disabled("Requires the started external Grape1Node server")
    @Test
    void transfer() {
        var senderPublicKey = genesisPubKeyHex;
        var senderPrivateKey = genesisPrivKeyHex;
        var recipient = "0xd7023dce93f477853427e1a46a65aea8b8847dd4";

        var signedTx = Grapech.newTxBuilder()
            .chainId(2)
            .type(TxBuilder.Type.PAYMENT)
            .senderPublicKey(senderPublicKey)
            .recipient(recipient)
            .amount(1_000_000L)
            .buildJson(senderPrivateKey);

        var rc = post(transactionsEndpoint, signedTx)
            .then()
            .assertThat()
            .statusCode(200)
            .body("executionStatus", is("SUCCESSFUL"))
            .extract().body().asString();
        log.info("response={}", rc);

    }

    @Disabled("Requires the started external Grape1Node server")
    @Test
    void publish() {
        var contractCode = "0x608060405234801561001057600080fd5b50610150806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80632e64cec11461003b5780636057361d14610059575b600080fd5b610043610075565b60405161005091906100d9565b60405180910390f35b610073600480360381019061006e919061009d565b61007e565b005b60008054905090565b8060008190555050565b60008135905061009781610103565b92915050565b6000602082840312156100b3576100b26100fe565b5b60006100c184828501610088565b91505092915050565b6100d3816100f4565b82525050565b60006020820190506100ee60008301846100ca565b92915050565b6000819050919050565b600080fd5b61010c816100f4565b811461011757600080fd5b5056fea26469706673582212209a159a4f3847890f10bfb87871a61eba91c5dbf5ee3cf6398207e292eee22a1664736f6c63430008070033";
        var senderPublicKey = genesisPubKeyHex;
        var senderPrivateKey = genesisPrivKeyHex;

        var account = retrieveAccount(genesisAddrHex);

        var signedTx = Grapech.newTxBuilder()
            .chainId(2)
            .type(TxBuilder.Type.PUBLISH_CONTRACT)
            .senderPublicKey(senderPublicKey)
            .nonce(account.nonce)//!!!! Set the real nonce that should be requested from the account
            .amount(0)
            .fuelLimit(70_000)
            .fuelPrice(5)
            .data(contractCode)
            .buildJson(senderPrivateKey);

        var rc = post(transactionsEndpoint, signedTx)
            .then()
            .assertThat()
            .statusCode(200)
            .body("executionStatus", is("SUCCESSFUL"))
            .extract().body().asString();
        log.info("response={}", rc);

    }

    @Disabled("Requires the started external Grape1Node server")
    @Test
    void callContract() {
        var callCode = HexUtils.parseHex("0x6057361d0000000000000000000000000000000000000000000123456789aabbccddeeff");
        var senderPublicKey = genesisPubKeyHex;
        var senderPrivateKey = genesisPrivKeyHex;

        var account = retrieveAccount(genesisAddrHex);

        var contractAddress = Grapech.createAddress(HexUtils.parseHex(genesisAddrHex), account.nonce - 1);

        var signedTx = Grapech.newTxBuilder()
            .chainId(2)
            .type(TxBuilder.Type.CALL_CONTRACT)
            .senderPublicKey(senderPublicKey)
            .recipient(contractAddress)
            .nonce(account.nonce)//!!!! Set the real nonce that should be requested from the account
            .amount(0)
            .fuelLimit(70_000)
            .fuelPrice(5)
            .data(callCode)
            .buildJson(senderPrivateKey);

        var rc = post(transactionsEndpoint, signedTx)
            .then()
            .assertThat()
            .statusCode(200)
            .body("executionStatus", is("SUCCESSFUL"))
            .extract().body().asString();
        log.info("response={}", rc);

    }

    @Disabled("Requires the started external Grape1Node server")
    @Test
    void estimatePublish() {
        var contractCode = "0x608060405234801561001057600080fd5b50610150806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80632e64cec11461003b5780636057361d14610059575b600080fd5b610043610075565b60405161005091906100d9565b60405180910390f35b610073600480360381019061006e919061009d565b61007e565b005b60008054905090565b8060008190555050565b60008135905061009781610103565b92915050565b6000602082840312156100b3576100b26100fe565b5b60006100c184828501610088565b91505092915050565b6100d3816100f4565b82525050565b60006020820190506100ee60008301846100ca565b92915050565b6000819050919050565b600080fd5b61010c816100f4565b811461011757600080fd5b5056fea26469706673582212209a159a4f3847890f10bfb87871a61eba91c5dbf5ee3cf6398207e292eee22a1664736f6c63430008070033";

        var message = Grapech.newMsgBuilder()
            .from(genesisAddrHex)
            .amount(0)
            .data(contractCode)
            .buildJson();

        var rc = post(estimateEndpoint, message)
            .then()
            .assertThat()
            .statusCode(200)
            .body("executionStatus", is("SUCCESSFUL"))
            .extract().body().asString();
        log.info("response={}", rc);

    }

    @Disabled("Requires the started external Grape1Node server")
    @Test
    void estimateCallContract() {
        var callCode = HexUtils.parseHex("0x6057361d0000000000000000000000000000000000000000000123456789aabbccddeeff");
        var sender = HexUtils.parseHex(genesisAddrHex);
        var account = retrieveAccount(genesisAddrHex);
        var contractAddress = Grapech.createAddress(HexUtils.parseHex(genesisAddrHex), account.nonce - 2);

        var message = Grapech.newMsgBuilder()
            //.from(sender)
            .to(contractAddress)
            .amount(0)
            .data(callCode)
            .buildJson();

        var rc = post(estimateEndpoint, message)
            .then()
            .assertThat()
            .statusCode(200)
            .body("executionStatus", is("SUCCESSFUL"))
            .extract().body().asString();
        log.info("response={}", rc);

    }

    private static Response post(String path, String message) {
        return given()
            .contentType(ContentType.JSON)
            .accept(ContentType.JSON)
            .header("Authorization", authHeader)
            .baseUri(baseUri)
            .when()
            .body(message)
            .post(path);
    }

    private static AccountResponse retrieveAccount(String address) {
        return get(accountsEndpoint, Map.of("address", address))
            .then()
            .extract()
            .body()
            .as(AccountResponse.class);
    }

    private static Response get(String path, Map<String, String> params) {
        return given()
            .contentType(ContentType.JSON)
            .accept(ContentType.JSON)
            .header("Authorization", authHeader)
            .baseUri(baseUri)
            .pathParams(params)
            .get(path);
    }

    private static class AccountResponse {
        @JsonProperty("balance")
        long balance;
        @JsonProperty("created")
        String created;
        @JsonProperty("id")
        String id;
        @JsonProperty("nonce")
        long nonce;
        @JsonProperty("publicKey")
        String publicKey;
    }
}
