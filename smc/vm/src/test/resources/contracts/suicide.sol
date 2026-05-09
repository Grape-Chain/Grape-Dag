// SPDX-License-Identifier: MIT
pragma solidity ^0.8.1;

contract Mortal {
    address public owner;

    // Initialize Contract: set owner
    constructor() {
        owner = msg.sender;
    }

    // Contract destructor
    function destroy() public {
        require(msg.sender == owner, "msg.sender is not the owner");
        selfdestruct(payable(owner));
    }
}
