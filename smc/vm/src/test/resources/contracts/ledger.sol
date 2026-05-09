//SPDX-License-Identifier: MIT

pragma solidity >=0.8.0 <0.9.0;

contract Ledger {
    struct Transaction {
        uint amount;
        uint timestamp;
    }

    struct Balance {
        uint totalBalance;
        uint numDeposits;
        mapping(uint => Transaction) deposits;
        uint numWithdrawals;
        mapping(uint => Transaction) withdrawals;
    }

    mapping(address => Balance) public  balances;

    function deposit() public payable {
        balances[msg.sender].totalBalance += msg.value;
        Transaction memory _deposit = Transaction(msg.value, block.timestamp);
        balances[msg.sender].deposits[
        balances[msg.sender].numDeposits] = _deposit;
        balances[msg.sender].numDeposits++;

    }

    function withdrawMoney(address payable _to, uint _amount) public {
        balances[msg.sender].totalBalance -= _amount;
        Transaction memory withdrawal = Transaction(_amount, block.timestamp);
        balances[msg.sender].withdrawals[
        balances[msg.sender].numWithdrawals] = withdrawal;
        balances[msg.sender].numWithdrawals++;
        _to.transfer(_amount);
    }

    function getDepositNum(address _from, uint _numDeposit) public view returns (Transaction memory) {
        return balances[_from].deposits[_numDeposit];
    }
}
