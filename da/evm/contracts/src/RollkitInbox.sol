// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

contract RollkitInbox {
    mapping(bytes32 => mapping(uint256 => bytes))
        public namespaceToDaHeightBlock;

    function SubmitBlock(bytes calldata data) public {
        namespaceToDaHeightBlock[bytes32("temp")][block.number] = data;
    }

    function RetrieveBlock(uint64 daHeight) public view returns (bytes memory) {
        return namespaceToDaHeightBlock[bytes32("temp")][uint256(daHeight)];
    }
}
