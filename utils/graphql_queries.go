package utils

import "github.com/machinebox/graphql"

const (
    RandomWordsRequestedQuery = `
        query MyQuery {
            randomWordsRequesteds(orderBy: blockTimestamp, orderDirection: desc, first: 50) {
                blockTimestamp
                roundInfo {
                    commitCount
                    validCommitCount
                    isRecovered
                    isFulfillExecuted
                }
                round
            }
        }`

    CommitCsQuery = `
        query MyQuery($round: String!, $msgSender: String!) {
            commitCs(where: {round: $round, msgSender: $msgSender}) {
                blockTimestamp
                commitVal
            }
        }`

    RecoveredDataQuery = `
        query MyQuery($round: String!) {
            recovereds(orderBy: blockTimestamp, orderDirection: asc, where: {round: $round}) {
                round
                blockTimestamp
                id
                msgSender
                omega
                roundInfo {
                    isRecovered
                }
            }
        }`

    CommitDataQuery = `
        query MyQuery($round: String!) {
            commitCs(where: {round: $round}) {
                round
                msgSender
                blockTimestamp
                commitIndex
                commitVal
                id
            }
        }`

    FulfillRandomnessDataQuery = `
        query GetFulfillRandomness($round: String!) {
            fulfillRandomnesses(where: {round: $round}) {
                msgSender
                blockTimestamp
                success
            }
        }`

    IsOperatorQuery = `
        query MyQuery($operator: String!) {
            operatorNumberChangeds(where: {operator: $operator}) {
                isOperator
            }
        }`
)

// GetRandomWordsRequestedRequest returns a GraphQL request for fetching random words requested.
func GetRandomWordsRequestedRequest() *graphql.Request {
    return graphql.NewRequest(RandomWordsRequestedQuery)
}

// GetCommitCsRequest returns a GraphQL request for fetching commitCs data.
func GetCommitCsRequest(round, msgSender string) *graphql.Request {
    req := graphql.NewRequest(CommitCsQuery)
    req.Var("round", round)
    req.Var("msgSender", msgSender)
    return req
}

// GetRecoveredDataRequest returns a GraphQL request for fetching recovered data.
func GetRecoveredDataRequest(round string) *graphql.Request {
    req := graphql.NewRequest(RecoveredDataQuery)
    req.Var("round", round)
    return req
}

// GetCommitDataRequest returns a GraphQL request for fetching commit data.
func GetCommitDataRequest(round string) *graphql.Request {
    req := graphql.NewRequest(CommitDataQuery)
    req.Var("round", round)
    return req
}

// GetFulfillRandomnessDataRequest returns a GraphQL request for fetching fulfill randomness data.
func GetFulfillRandomnessDataRequest(round string) *graphql.Request {
    req := graphql.NewRequest(FulfillRandomnessDataQuery)
    req.Var("round", round)
    return req
}

// GetIsOperatorRequest returns a GraphQL request for checking if an address is an operator.
func GetIsOperatorRequest(operator string) *graphql.Request {
    req := graphql.NewRequest(IsOperatorQuery)
    req.Var("operator", operator)
    return req
}
