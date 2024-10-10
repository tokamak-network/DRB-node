// SPDX-License-Identifier: MIT
pragma solidity ^0.8.26;

import {DRBCoordinatorStorage} from "./DRBCoordinatorStorage.sol";
import {ReentrancyGuardTransient} from "./utils/ReentrancyGuardTransient.sol";
import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {OptimismL1Fees} from "./OptimismL1Fees.sol";
import {DRBConsumerBase} from "./DRBConsumerBase.sol";
import {IDRBCoordinator} from "./interfaces/IDRBCoordinator.sol";

/// @title DRBCoordinator, distributed random beacon coordinator, using commit-reveal scheme
/// @author Justin G

contract DRBCoordinator is
Ownable,
ReentrancyGuardTransient,
IDRBCoordinator,
DRBCoordinatorStorage,
OptimismL1Fees
{
    /// *** Functions ***
    constructor(
        uint256 activationThreshold,
        uint256 flatFee,
        uint256 compensateAmount
    ) Ownable(msg.sender) {
        s_activationThreshold = activationThreshold;
        s_flatFee = flatFee;
        s_compensateAmount = compensateAmount;
        s_activatedOperators.push(address(0)); // dummy data
    }

    /// ***
    /// ** Consumer Interface **
    function requestRandomNumber(
        uint32 callbackGasLimit
    ) external payable nonReentrant returns (uint256 round) {
        require(
            callbackGasLimit <= MAX_CALLBACK_GAS_LIMIT,
            ExceedCallbackGasLimit()
        );
        require(s_activatedOperators.length > 2, NotEnoughActivatedOperators());
        require(
            msg.value >= _calculateRequestPrice(callbackGasLimit, tx.gasprice),
            InsufficientAmount()
        );
        unchecked {
            round = s_nextRound++;
        }
        uint256 requestAndRefundCost = _calculateGetRequestAndRefundCost(
            tx.gasprice
        );
        uint256 minDepositForThisRound = _calculateMinDepositForOneRound(
            callbackGasLimit,
            tx.gasprice
        ) + requestAndRefundCost;
        s_requestInfo[round] = RequestInfo({
            consumer: msg.sender,
            requestedTime: block.timestamp,
            cost: msg.value,
            callbackGasLimit: callbackGasLimit,
            minDepositForOperator: minDepositForThisRound,
            requestAndRefundCost: requestAndRefundCost
        });
        address[] memory activatedOperators;
        s_activatedOperatorsAtRound[
        round
        ] = activatedOperators = s_activatedOperators;
        uint256 activatedOperatorsLength = activatedOperators.length;
        uint256 i = 1;
        mapping(address => uint256)
        storage activatedOperatorOrderAtRound = s_activatedOperatorOrderAtRound[
                    round
            ];
        uint256 activationThreshold = s_activationThreshold;
        do {
            address operator = activatedOperators[i];
            activatedOperatorOrderAtRound[operator] = i;
            uint256 activatedOperatorIndex = s_activatedOperatorOrder[operator];
            if (
                (s_depositAmount[operator] -= minDepositForThisRound) <
                activationThreshold
            ) _deactivate(activatedOperatorIndex, operator);
            unchecked {
                ++i;
            }
        } while (i < activatedOperatorsLength);
        emit RandomNumberRequested(round, activatedOperators);
    }

    /// @dev refund the cost of the request
    /// @param round the request id
    /// Note: condition for refund
    /// 1. A few minutes have passed without any commit after random number requested
    /// 2. CommitPhase is over and there are less than 2 commits
    /// 3. RevealPhase is over and at least one person hasn't revealed.
    function getRefund(uint256 round) external nonReentrant {
        require(msg.sender == s_requestInfo[round].consumer, NotConsumer());
        uint256 ruleNum = 3;
        uint256 commitEndTime = s_roundInfo[round].commitEndTime;
        uint256 commitLength = s_commits[round].length;
        uint256 revealLength = s_reveals[round].length;
        if (
            block.timestamp > s_requestInfo[round].requestedTime + MAX_WAIT &&
            commitLength == 0
        ) ruleNum = 0;
        else if (commitLength > 0) {
            if (commitLength < 2 && block.timestamp > commitEndTime)
                ruleNum = 1;
            else if (
                block.timestamp > commitEndTime + REVEAL_DURATION &&
                revealLength < commitLength
            ) ruleNum = 2;
        }
        require(ruleNum != 3, NotRefundable());

        uint256 activatedOperatorsAtRoundLength = s_activatedOperatorsAtRound[
                    round
            ].length - 1;

        if (ruleNum == 0) {
            uint256 totalSlashAmount = activatedOperatorsAtRoundLength *
                                s_requestInfo[round].minDepositForOperator;
            payable(msg.sender).transfer(
                totalSlashAmount + s_requestInfo[round].cost
            );
        } else {
            uint256 requestRefundTxCostAndCompensateAmount = s_requestInfo[
                        round
                ].requestAndRefundCost + s_compensateAmount;
            uint256 refundAmount = s_requestInfo[round].cost +
                        requestRefundTxCostAndCompensateAmount;
            uint256 minDepositAtRound = s_requestInfo[round]
                .minDepositForOperator;
            uint256 activationThreshold = s_activationThreshold;

            if (ruleNum == 1) {
                uint256 returnAmountForCommitted = minDepositAtRound +
                    (((activatedOperatorsAtRoundLength - commitLength) *
                    minDepositAtRound -
                        requestRefundTxCostAndCompensateAmount) / commitLength);
                for (
                    uint256 i = 1;
                    i <= activatedOperatorsAtRoundLength;
                    i = _unchecked_inc(i)
                ) {
                    address operator = s_activatedOperatorsAtRound[round][i];
                    if (s_commitOrder[round][operator] != 0) {
                        _checkAndActivateIfNotForceDeactivated(
                            s_activatedOperatorOrder[operator],
                            s_depositAmount[
                            operator
                            ] += returnAmountForCommitted,
                            activationThreshold,
                            operator
                        );
                    }
                }
            } else {
                uint256 returnAmountForRevealed = minDepositAtRound +
                    (((commitLength - revealLength) *
                    minDepositAtRound -
                        requestRefundTxCostAndCompensateAmount) / revealLength);
                for (
                    uint256 i = 1;
                    i <= activatedOperatorsAtRoundLength;
                    i = _unchecked_inc(i)
                ) {
                    address operator = s_activatedOperatorsAtRound[round][i];
                    if (s_revealOrder[round][operator] != 0) {
                        _checkAndActivateIfNotForceDeactivated(
                            s_activatedOperatorOrder[operator],
                            s_depositAmount[
                            operator
                            ] += returnAmountForRevealed,
                            activationThreshold,
                            operator
                        );
                    }
                }
            }
            payable(msg.sender).transfer(refundAmount);
        }
        emit Refund(round);
    }

    function calculateRequestPrice(
        uint256 callbackGasLimit
    ) external view returns (uint256) {
        return _calculateRequestPrice(callbackGasLimit, tx.gasprice);
    }

    function estimateRequestPrice(
        uint256 callbackGasLimit,
        uint256 gasPrice
    ) external view returns (uint256) {
        return _calculateRequestPrice(callbackGasLimit, gasPrice);
    }

    function estimateMinDepositForOneRound(
        uint256 callbackGasLimit,
        uint256 gasPrice
    ) external view returns (uint256) {
        return
            _calculateMinDepositForOneRound(callbackGasLimit, gasPrice) +
            _calculateGetRequestAndRefundCost(gasPrice);
    }

    function _checkAndActivateIfNotForceDeactivated(
        uint256 activatedOperatorIndex,
        uint256 updatedDepositAmount,
        uint256 minDepositForThisRound,
        address operator
    ) private {
        if (
            activatedOperatorIndex == 0 &&
            updatedDepositAmount >= minDepositForThisRound &&
            !s_forceDeactivated[operator]
        ) {
            _activate(operator);
        }
    }

    /// @dev 2 commits, 2 reveals
    function _calculateRequestPrice(
        uint256 callbackGasLimit,
        uint256 gasPrice
    ) private view returns (uint256) {
        return
            (((gasPrice * (callbackGasLimit + TWOCOMMIT_TWOREVEAL_GASUSED)) *
                (s_premiumPercentage + 100)) / 100) +
            s_flatFee +
            _getL1CostWeiForCalldataSize(
                TWOCOMMIT_TWOREVEAL_CALLDATA_BYTES_SIZE
            );
    }

    function _calculateMinDepositForOneRound(
        uint256 callbackGasLimit,
        uint256 gasPrice
    ) private view returns (uint256) {
        return
            (((gasPrice * (callbackGasLimit + ONECOMMIT_ONEREVEAL_GASUSED)) *
                (s_premiumPercentage + 100)) / 100) +
            s_flatFee +
            _getL1CostWeiForCalldataSize(
                ONECOMMIT_ONEREVEAL_CALLDATA_BYTES_SIZE
            ) +
            s_compensateAmount;
    }

    function _calculateGetRequestAndRefundCost(
        uint256 gasPrice
    ) private view returns (uint256) {
        return
            (((gasPrice * MAX_REQUEST_REFUND_GASUSED) *
                (s_premiumPercentage + 100)) / 100) +
            _getL1CostWeiForCalldataSize(REQUEST_REFUND_CALLDATA_BYTES_SIZE);
    }

    /// ***
    /// ** Operator(Node) Interface **

    function commit(uint256 round, bytes32 a) external {
        address[]
        storage activatedOperatorsAtRound = s_activatedOperatorsAtRound[
                    round
            ];
        require(
            activatedOperatorsAtRound[
            s_activatedOperatorOrderAtRound[round][msg.sender]
            ] == msg.sender,
            WasNotActivated()
        );
        bytes32[] storage commits = s_commits[round];
        RoundInfo storage roundInfo = s_roundInfo[round];
        mapping(address => uint256) storage commitOrder = s_commitOrder[round];
        uint256 commitLength = commits.length;
        if (commitLength == 0) {
            roundInfo.commitEndTime = block.timestamp + COMMIT_DURATION;
        } else {
            require(
                block.timestamp <= roundInfo.commitEndTime,
                CommitPhaseOver()
            );
            require(commitOrder[msg.sender] == 0, AlreadyCommitted());
        }
        commits.push(a);
        unchecked {
            ++commitLength;
        }
        commitOrder[msg.sender] = commitLength;
        if (commitLength == activatedOperatorsAtRound.length - 1) {
            roundInfo.commitEndTime = block.timestamp;
        }
        emit Commit(msg.sender, round);
    }

    function reveal(uint256 round, bytes32 s) external {
        uint256 commitOrder = s_commitOrder[round][msg.sender];
        require(commitOrder != 0, NotCommitted());
        mapping(address => uint256) storage revealOrder = s_revealOrder[round];
        require(revealOrder[msg.sender] == 0, AlreadyRevealed());
        RoundInfo storage roundInfo = s_roundInfo[round];
        bytes32[] storage commits = s_commits[round];
        bytes32[] storage reveals = s_reveals[round];
        uint256 commitEndTime = roundInfo.commitEndTime;
        uint256 commitLength = commits.length;
        require(
            (block.timestamp > commitEndTime &&
                block.timestamp <= commitEndTime + REVEAL_DURATION),
            NotRevealPhase()
        );
        require(
            keccak256(abi.encodePacked(s)) == commits[commitOrder - 1],
            RevealValueMismatch()
        );
        reveals.push(s);
        uint256 revealLength = revealOrder[msg.sender] = reveals.length;
        if (revealLength == commitLength) {
            uint256 randomNumber = uint256(
                keccak256(abi.encodePacked(reveals))
            );
            roundInfo.randomNumber = randomNumber;
            RequestInfo storage requestInfo = s_requestInfo[round];
            bool success = _call(
                requestInfo.consumer,
                abi.encodeWithSelector(
                    DRBConsumerBase.rawFulfillRandomWords.selector,
                    round,
                    randomNumber
                ),
                requestInfo.callbackGasLimit
            );
            roundInfo.fulfillSucceeded = success;
            uint256 minDepositForThisRound = requestInfo.minDepositForOperator;
            uint256 minDepositWithReward = requestInfo.cost /
                        revealLength +
                        minDepositForThisRound;
            uint256 activationThreshold = s_activationThreshold;
            uint256 activatedOperatorsAtRoundLength = s_activatedOperatorsAtRound[
                        round
                ].length - 1;
            for (
                uint256 i = 1;
                i <= activatedOperatorsAtRoundLength;
                i = _unchecked_inc(i)
            ) {
                address operator = s_activatedOperatorsAtRound[round][i];
                _checkAndActivateIfNotForceDeactivated(
                    s_activatedOperatorOrder[operator],
                    s_depositAmount[operator] += (
                        revealOrder[operator] != 0
                            ? minDepositWithReward
                            : minDepositForThisRound
                    ),
                    activationThreshold,
                    operator
                );
            }
        }
        emit Reveal(msg.sender, round);
    }

    function deposit() external payable nonReentrant {
        _deposit();
    }

    function depositAndActivate() external payable nonReentrant {
        _deposit();
        _activate(msg.sender);
    }

    function withdraw(uint256 amount) external nonReentrant {
        s_depositAmount[msg.sender] -= amount;
        uint256 activatedOperatorIndex = s_activatedOperatorOrder[msg.sender];
        if (
            activatedOperatorIndex != 0 &&
            s_depositAmount[msg.sender] < s_activationThreshold
        ) _deactivate(activatedOperatorIndex, msg.sender);
        payable(msg.sender).transfer(amount);
    }

    function activate() external nonReentrant {
        require(
            s_depositAmount[msg.sender] >= s_activationThreshold,
            InsufficientDeposit()
        );
        if (s_forceDeactivated[msg.sender])
            s_forceDeactivated[msg.sender] = false;
        _activate(msg.sender);
    }

    function deactivate() external nonReentrant {
        require(
            s_forceDeactivated[msg.sender] == false,
            AlreadyForceDeactivated()
        );
        s_forceDeactivated[msg.sender] = true;
        uint256 activatedOperatorIndex = s_activatedOperatorOrder[msg.sender];
        if (activatedOperatorIndex != 0)
            _deactivate(activatedOperatorIndex, msg.sender);
    }

    function _activate(address operator) private {
        require(s_activatedOperatorOrder[operator] == 0, AlreadyActivated());
        uint256 activatedOperatorLength = s_activatedOperators.length;
        require(
            activatedOperatorLength <= MAX_ACTIVATED_OPERATORS,
            ACTIVATED_OPERATORS_LIMIT_REACHED()
        );
        s_activatedOperatorOrder[operator] = activatedOperatorLength;
        s_activatedOperators.push(operator);
        emit Activated(operator);
    }

    function _deposit() private {
        uint256 totalAmount = s_depositAmount[msg.sender] + msg.value;
        require(totalAmount >= s_activationThreshold, InsufficientAmount());
        s_depositAmount[msg.sender] = totalAmount;
    }

    function _deactivate(
        uint256 activatedOperatorIndex,
        address operator
    ) private {
        address lastOperator = s_activatedOperators[
            s_activatedOperators.length - 1
            ];
        s_activatedOperators[activatedOperatorIndex] = lastOperator;
        s_activatedOperators.pop();
        s_activatedOperatorOrder[lastOperator] = activatedOperatorIndex;
        delete s_activatedOperatorOrder[operator];
        emit DeActivated(operator);
    }

    function _call(
        address target,
        bytes memory data,
        uint256 callbackGasLimit
    ) private returns (bool success) {
        assembly {
            let g := gas()
        // Compute g -= GAS_FOR_CALL_EXACT_CHECK and check for underflow
        // The gas actually passed to the callee is min(gasAmount, 63//64*gas available)
        // We want to ensure that we revert if gasAmount > 63//64*gas available
        // as we do not want to provide them with less, however that check itself costs
        // gas. GAS_FOR_CALL_EXACT_CHECK ensures we have at least enough gas to be able to revert
        // if gasAmount > 63//64*gas available.
            if lt(g, GAS_FOR_CALL_EXACT_CHECK) {
                revert(0, 0)
            }
            g := sub(g, GAS_FOR_CALL_EXACT_CHECK)
        // if g - g//64 <= gas
        // we subtract g//64 because of EIP-150
            g := sub(g, div(g, 64))
            if iszero(gt(sub(g, div(g, 64)), callbackGasLimit)) {
                revert(0, 0)
            }
        // solidity calls check that a contract actually exists at the destination, so we do the same
            if iszero(extcodesize(target)) {
                revert(0, 0)
            }
        // call and return whether we succeeded. ignore return data
        // call(gas, addr, value, argsOffset,argsLength,retOffset,retLength)
            success := call(
                callbackGasLimit,
                target,
                0,
                add(data, 0x20),
                mload(data),
                0,
                0
            )
        }
        return success;
    }

    function _unchecked_inc(uint256 a) private pure returns (uint256) {
        unchecked {
            return a + 1;
        }
    }

    /// ***
    /// ** Owner Interface **
    function setPremiumPercentage(
        uint256 premiumPercentage
    ) external onlyOwner {
        s_premiumPercentage = premiumPercentage;
    }

    function setFlatFee(uint256 flatFee) external onlyOwner {
        s_flatFee = flatFee;
    }

    function setActivationThreshold(
        uint256 activationThreshold
    ) external onlyOwner {
        s_activationThreshold = activationThreshold;
    }

    function setCompensations(uint256 compensateAmount) external onlyOwner {
        s_compensateAmount = compensateAmount;
    }
}