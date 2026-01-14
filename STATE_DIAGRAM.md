# DRB System State Diagram

## System State Flow (Corrected Based on Official Diagram)

```mermaid
stateDiagram-v2
    [*] --> COMPLETED : System Deployment
    
    COMPLETED --> IN_PROGRESS : requestRandomNumber()
    IN_PROGRESS --> COMPLETED : Success Path
    IN_PROGRESS --> HALTED : Failure Path
    HALTED --> [*] : Manual Recovery Required
    
    state IN_PROGRESS {
        [*] --> OFF_CHAIN_SUBMISSION
        
        OFF_CHAIN_SUBMISSION --> TIMER_CHECK_1 : Collection Period Ends
        
        TIMER_CHECK_1 --> MERKLE_ROOT_SUBMISSION : All Commits Received
        TIMER_CHECK_1 --> DISPUTE_CV : Missing Commits
        TIMER_CHECK_1 --> HALT_EARLY : failToRequestSubmitCvOrSubmitMerkleRoot()
        
        DISPUTE_CV --> CV_SUBMISSION : requestToSubmitCv()
        CV_SUBMISSION --> MERKLE_ROOT_SUBMISSION : CVs Submitted  
        CV_SUBMISSION --> HALT_CV_FAIL : failToSubmitCv()
        CV_SUBMISSION --> HALT_MERKLE_AFTER_DISPUTE : failToSubmitMerkleRootAfterDispute()
        
        MERKLE_ROOT_SUBMISSION --> TIMER_CHECK_2 : submitMerkleRoot()
        
        TIMER_CHECK_2 --> DIRECT_GENERATION : Can Generate Directly
        TIMER_CHECK_2 --> DISPUTE_CO : Need More Data
        TIMER_CHECK_2 --> HALT_S_FAIL : failToRequestSOrGenerateRandomNumber()
        
        DISPUTE_CO --> CO_SUBMISSION : requestToSubmitCo()
        CO_SUBMISSION --> TIMER_CHECK_3 : COs Collected
        CO_SUBMISSION --> HALT_CO_FAIL : failToSubmitCo()
        
        TIMER_CHECK_3 --> DIRECT_GENERATION : Sufficient COs
        TIMER_CHECK_3 --> DISPUTE_S : Need Secrets
        
        DISPUTE_S --> S_SUBMISSION : requestToSubmitS()
        S_SUBMISSION --> DIRECT_GENERATION : Secrets Submitted
        S_SUBMISSION --> HALT_S_ALL_FAIL : failToSubmitAllS()
        
        DIRECT_GENERATION --> CALLBACK : generateRandomNumber()
        CALLBACK --> [*] : Consumer Callback Success
        
        HALT_EARLY --> [*]
        HALT_CV_FAIL --> [*]
        HALT_MERKLE_AFTER_DISPUTE --> [*]
        HALT_CO_FAIL --> [*]
        HALT_S_FAIL --> [*]
        HALT_S_ALL_FAIL --> [*]
    }
```

## Backend Node State Flow

```mermaid
stateDiagram-v2
    [*] --> NODE_STARTUP
    
    state NODE_STARTUP {
        [*] --> INITIALIZE_P2P
        INITIALIZE_P2P --> CONNECT_DATABASE : P2P Success
        INITIALIZE_P2P --> ERROR_P2P : P2P Failure
        
        CONNECT_DATABASE --> START_MONITORING : DB Success
        CONNECT_DATABASE --> ERROR_DB : DB Failure
        
        START_MONITORING --> READY : Monitoring Started
        
        ERROR_P2P --> [*]
        ERROR_DB --> [*]
    }
    
    NODE_STARTUP --> READY : Successful Initialization
    NODE_STARTUP --> FAILED : Initialization Error
    
    READY --> ROUND_ACTIVE : Round Started
    ROUND_ACTIVE --> READY : Round Completed
    ROUND_ACTIVE --> ERROR_RECOVERY : Node/Network Failure
    
    ERROR_RECOVERY --> READY : Recovery Successful
    ERROR_RECOVERY --> FAILED : Recovery Failed
    
    FAILED --> [*] : Manual Intervention Required
    
    state ROUND_ACTIVE {
        [*] --> LEADER_ORCHESTRATION
        [*] --> REGULAR_PARTICIPATION
        
        state LEADER_ORCHESTRATION {
            [*] --> COLLECT_COMMITS
            COLLECT_COMMITS --> BUILD_MERKLE : All Commits Received
            COLLECT_COMMITS --> TIMEOUT_HANDLING : Timeout Occurred
            
            BUILD_MERKLE --> SUBMIT_MERKLE : Tree Built
            BUILD_MERKLE --> ERROR_BUILD : Build Failed
            
            SUBMIT_MERKLE --> GENERATE_RANDOM : Success
            SUBMIT_MERKLE --> ERROR_SUBMIT : Transaction Failed
            
            TIMEOUT_HANDLING --> INITIATE_DISPUTE : Trigger CV Request
            INITIATE_DISPUTE --> COLLECT_CVS : CV Request Sent
            COLLECT_CVS --> BUILD_MERKLE : CVs Collected
            
            GENERATE_RANDOM --> [*] : Success
            ERROR_BUILD --> [*]
            ERROR_SUBMIT --> [*]
        }
        
        state REGULAR_PARTICIPATION {
            [*] --> GENERATE_COMMIT
            GENERATE_COMMIT --> SEND_COMMIT : Commit Generated
            SEND_COMMIT --> WAIT_PHASE : Commit Sent
            SEND_COMMIT --> RETRY_SEND : Send Failed
            
            RETRY_SEND --> SEND_COMMIT : Retry
            RETRY_SEND --> ERROR_SEND : Max Retries
            
            WAIT_PHASE --> SUBMIT_CV : CV Requested
            WAIT_PHASE --> SUBMIT_CO : CO Requested
            WAIT_PHASE --> SUBMIT_S : S Requested
            WAIT_PHASE --> [*] : Round Complete
            
            SUBMIT_CV --> WAIT_PHASE : CV Submitted
            SUBMIT_CO --> WAIT_PHASE : CO Submitted
            SUBMIT_S --> WAIT_PHASE : S Submitted
            
            ERROR_SEND --> [*]
        }
    }
```

## Network Communication State Flow

```mermaid
stateDiagram-v2
    [*] --> NETWORK_INIT
    
    NETWORK_INIT --> PEER_DISCOVERY : P2P Started
    PEER_DISCOVERY --> CONNECTED : Peers Found
    PEER_DISCOVERY --> ISOLATED : No Peers Found
    
    CONNECTED --> STABLE_COMM : Communication Established
    STABLE_COMM --> MESSAGE_EXCHANGE : Active Round
    
    MESSAGE_EXCHANGE --> STABLE_COMM : Message Success
    MESSAGE_EXCHANGE --> NETWORK_ISSUE : Message Failed
    
    NETWORK_ISSUE --> RECONNECTING : Temporary Issue
    NETWORK_ISSUE --> PARTITIONED : Persistent Issue
    
    RECONNECTING --> STABLE_COMM : Recovery Success
    RECONNECTING --> PARTITIONED : Recovery Failed
    
    PARTITIONED --> ISOLATED : All Peers Lost
    PARTITIONED --> RECONNECTING : Partial Recovery
    
    ISOLATED --> PEER_DISCOVERY : Retry Connection
    
    state MESSAGE_EXCHANGE {
        [*] --> COMMIT_PHASE
        COMMIT_PHASE --> CV_PHASE : CV Requested
        COMMIT_PHASE --> COMPLETION : Direct Success
        
        CV_PHASE --> CO_PHASE : CO Requested
        CV_PHASE --> COMPLETION : CV Success
        
        CO_PHASE --> S_PHASE : S Requested
        CO_PHASE --> COMPLETION : CO Success
        
        S_PHASE --> COMPLETION : S Success
        
        COMPLETION --> [*]
    }
```

## Error Recovery Flow

```mermaid
stateDiagram-v2
    [*] --> ERROR_DETECTED
    
    ERROR_DETECTED --> CLASSIFY_ERROR : Error Analysis
    
    CLASSIFY_ERROR --> TRANSIENT_ERROR : Temporary Issue
    CLASSIFY_ERROR --> PERMANENT_ERROR : Persistent Issue
    CLASSIFY_ERROR --> CRITICAL_ERROR : System Failure
    
    TRANSIENT_ERROR --> RETRY_OPERATION : Automatic Retry
    RETRY_OPERATION --> SUCCESS_RECOVERY : Operation Success
    RETRY_OPERATION --> ESCALATE_ERROR : Max Retries Reached
    
    PERMANENT_ERROR --> GRACEFUL_DEGRADATION : Reduce Functionality
    PERMANENT_ERROR --> ESCALATE_ERROR : Cannot Degrade
    
    CRITICAL_ERROR --> SYSTEM_HALT : Immediate Stop
    SYSTEM_HALT --> MANUAL_RECOVERY : Human Intervention
    
    ESCALATE_ERROR --> GRACEFUL_DEGRADATION : Attempt Degradation
    ESCALATE_ERROR --> SYSTEM_HALT : Cannot Continue
    
    GRACEFUL_DEGRADATION --> MONITORING : Reduced Service
    MONITORING --> SUCCESS_RECOVERY : Issue Resolved
    MONITORING --> ESCALATE_ERROR : Degradation Failed
    
    SUCCESS_RECOVERY --> [*] : Normal Operation
    MANUAL_RECOVERY --> [*] : System Restored
```

## Legend

### State Types
- **🟢 Success States**: Normal operation paths
- **🟡 Warning States**: Degraded operation or retry scenarios  
- **🔴 Error States**: Failure states requiring intervention
- **⭕ Terminal States**: End states requiring restart/recovery

### Transition Types
- **Solid Lines**: Normal flow transitions
- **Dashed Lines**: Error/exception transitions
- **Dotted Lines**: Recovery/retry transitions

### Coverage Status
- **✅ Tested**: State and transitions have test coverage
- **⚠️ Partial**: Some aspects tested, gaps exist
- **❌ Missing**: No test coverage for state/transition