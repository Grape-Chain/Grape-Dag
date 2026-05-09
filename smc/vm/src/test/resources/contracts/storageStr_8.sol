// SPDX-License-Identifier: GPL-3.0

pragma solidity ^0.8.12;

contract StorageStr {

    string private name1;
    string private name2;

    constructor(string memory str1, string memory str2) {
        name1 = str1;
        name2 = str2;

    }

    function store1(string memory str) public {
        name1 = str;
    }

    function store2(string memory str) public {
        name2 = str;
    }

    function retrieve1() public view returns (string memory){
        return name1;
    }

    function retrieve2() public view returns (string memory){
        return name2;
    }
}
