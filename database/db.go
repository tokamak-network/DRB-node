package database

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-pg/pg/v10"
	"github.com/gobuffalo/packr/v2"
	_ "github.com/lib/pq"
	migrate "github.com/rubenv/sql-migrate"
	"github.com/tokamak-network/DRB-node/utils"
)

var migrations *migrate.PackrMigrationSource

var (
	dbClient *pg.DB
	once     sync.Once
)

func init() {
	migrations = &migrate.PackrMigrationSource{
		Box: packr.New("drb-db-migrations", "./migrations"),
	}
	ms, err := migrations.FindMigrations()
	if err != nil {
		panic(err)
	}
	if len(ms) == 0 {
		panic(fmt.Errorf("no SQL migrations found"))
	}
}

// MigrationsUp runs the SQL migration up
func MigrationsUp(db *sql.DB) error {
	nMigrations, err := migrate.Exec(db, "postgres", migrations, migrate.Up)
	if err != nil {
		return err
	}
	fmt.Printf("successfully ran migration up: %d \n", nMigrations)
	return nil
}

// MigrationsDown runs the SQL migration down
func MigrationsDown(db *sql.DB) error {
	_, err := migrate.Exec(db, "postgres", migrations, migrate.Down)
	if err != nil {
		return err
	}
	fmt.Println("successfully ran migration down")
	return nil
}

// ConnectSQLDB connects to the SQL DB
func InitSQLDB(port int, host, user, password, name string) error {
	// Establish Connection
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", user, password, host, port, name)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect SQL DB, %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		panic(fmt.Errorf("error pinging database for migration: %v", err))
	}

	// Run DB migrations
	if err := MigrationsUp(db); err != nil {
		return fmt.Errorf("failed to run migration up, %v", err)
	}

	once.Do(func() {
		opts := &pg.Options{
			User:                  user,
			Password:              password,
			Database:              name,
			Addr:                  fmt.Sprintf("%s:%d", host, port),
			MinIdleConns:          10,
			MaxConnAge:            10 * time.Minute,
			IdleTimeout:           5 * time.Minute,
			PoolSize:              10,
			PoolTimeout:           5 * time.Minute,
			IdleCheckFrequency:    1 * time.Minute,
			MaxRetries:            3,
			RetryStatementTimeout: true,
		}
		dbClient = pg.Connect(opts)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := dbClient.Ping(ctx); err != nil {
			dbClient = nil
			panic(fmt.Sprintf("Error connecting main DB client (go-pg): %v", err))
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := GetDB().Ping(ctx); err != nil {
		panic(fmt.Sprintf("Error pinging main DB client after initialization: %v", err))
	}

	log.Println("Database initialised successfully.")
	return nil
}

// GetDB returns the singleton database client instance.
// It assumes InitialiseDB has been called successfully at least once.
func GetDB() *pg.DB {
	if dbClient == nil {
		panic("database client is not initialized. Call InitialiseDB first.")
	}
	return dbClient
}

func AddNodeInfo(nodeInfo *utils.NodeInfo) error {
	node := &NodeInfoScheme{
		IP:         nodeInfo.IP,
		Port:       nodeInfo.Port,
		PeerID:     nodeInfo.PeerID,
		EOAAddress: nodeInfo.EOAAddress,
	}

	_, err := GetDB().Model(node).Insert()
	if err != nil {
		return err
	}

	return nil
}

func UpdateNodeInfo(nodeInfo *utils.NodeInfo) error {
	node := &NodeInfoScheme{
		IP:         nodeInfo.IP,
		Port:       nodeInfo.Port,
		PeerID:     nodeInfo.PeerID,
		EOAAddress: nodeInfo.EOAAddress,
	}

	_, err := GetDB().Model(node).WherePK().Insert()
	if err != nil {
		return err
	}

	return nil
}

func GetNodeInfos() ([]*utils.NodeInfo, error) {
	var nodes []NodeInfoScheme
	err := GetDB().Model(&nodes).Select()
	if err != nil {
		return nil, err
	}

	nodeInfos := make([]*utils.NodeInfo, 0, len(nodes))
	for _, node := range nodes {
		nodeInfo := &utils.NodeInfo{
			IP:         node.IP,
			Port:       node.Port,
			PeerID:     node.PeerID,
			EOAAddress: node.EOAAddress,
		}

		nodeInfos = append(nodeInfos, nodeInfo)
	}

	return nodeInfos, nil
}

func AddLeaderCommit(commitData *utils.LeaderCommitData) error {
	leaderCommit := LeaderCommitScheme{
		UniqueKey:             commitData.UniqueKey,
		Round:                 commitData.Round,
		TrialNum:              commitData.TrialNum,
		EOAAddress:            commitData.EOAAddress,
		Cvs:                   commitData.Cvs[:],
		Cos:                   commitData.Cos[:],
		SecretValue:           commitData.SecretValue[:],
		SignR:                 commitData.Sign.R,
		SignS:                 commitData.Sign.S,
		SignV:                 commitData.Sign.V,
		SubmitMerkleRootDone:  commitData.SubmitMerkleRootDone,
		RandomNumberGenerated: commitData.RandomNumberGenerated,
		CreatedAt:             time.Now().Unix(),
	}

	// Convert byte arrays to hex strings
	if len(leaderCommit.Cvs) > 0 {
		leaderCommit.CvsHex = hex.EncodeToString(leaderCommit.Cvs)
	}
	if len(leaderCommit.Cos) > 0 {
		leaderCommit.CosHex = hex.EncodeToString(leaderCommit.Cos)
	}
	if len(leaderCommit.SecretValue) > 0 {
		leaderCommit.SecretValueHex = hex.EncodeToString(leaderCommit.SecretValue)
	}

	_, err := GetDB().Model(&leaderCommit).Insert()
	if err != nil {
		return err
	}

	return nil
}

func GetLeaderCommitByRoundAndEoaAddr(round, trailNum, uniqueKey, eoaAddr string) (*utils.LeaderCommitData, error) {
	var leaderCommit LeaderCommitScheme
	err := GetDB().Model(&leaderCommit).
		Where("unique_key = ? AND eoa_address = ?", uniqueKey, eoaAddr).
		Limit(1).
		Select()
	if err != nil {
		return nil, err
	}

	sign := &utils.SignInfo{
		R: leaderCommit.SignR,
		S: leaderCommit.SignS,
		V: leaderCommit.SignV,
	}

	// Map LeaderCommit to utils.LeaderCommitData
	leaderCommitData := &utils.LeaderCommitData{
		UniqueKey:             uniqueKey,
		Round:                 leaderCommit.Round,
		TrialNum:              trailNum,
		EOAAddress:            leaderCommit.EOAAddress,
		Cvs:                   utils.ConvertByteArray(leaderCommit.Cvs),
		CvsHex:                leaderCommit.CvsHex,
		Cos:                   utils.ConvertByteArray(leaderCommit.Cos),
		CosHex:                leaderCommit.CosHex,
		SecretValue:           utils.ConvertByteArray(leaderCommit.SecretValue),
		SecretValueHex:        leaderCommit.SecretValueHex,
		Sign:                  *sign,
		SubmitMerkleRootDone:  leaderCommit.SubmitMerkleRootDone,
		RandomNumberGenerated: leaderCommit.RandomNumberGenerated,
		CreatedAt:             leaderCommit.CreatedAt,
	}

	return leaderCommitData, nil
}

func GetLeaderCommitsByRoundAndTrialNum(round string, trialNum string) ([]*utils.LeaderCommitData, error) {
	// Slice to hold DB model results
	var leaderCommitModels []LeaderCommitScheme
	uniqueKey := utils.GetUniqueKey(round, trialNum)

	err := GetDB().Model(&leaderCommitModels).
		Where("unique_key = ?", uniqueKey).
		Select()
	if err != nil {
		return nil, err
	}

	// Map DB models to utils.LeaderCommitData slice
	leaderCommits := make([]*utils.LeaderCommitData, 0, len(leaderCommitModels))
	for _, model := range leaderCommitModels {
		sign := &utils.SignInfo{
			R: model.SignR,
			S: model.SignS,
			V: model.SignV,
		}

		commitData := &utils.LeaderCommitData{
			UniqueKey:             model.UniqueKey,
			Round:                 round,
			TrialNum:              trialNum,
			EOAAddress:            model.EOAAddress,
			Cvs:                   utils.ConvertByteArray(model.Cvs),
			CvsHex:                model.CvsHex,
			Cos:                   utils.ConvertByteArray(model.Cos),
			CosHex:                model.CosHex,
			SecretValue:           utils.ConvertByteArray(model.SecretValue),
			SecretValueHex:        model.SecretValueHex,
			Sign:                  *sign,
			SubmitMerkleRootDone:  model.SubmitMerkleRootDone,
			RandomNumberGenerated: model.RandomNumberGenerated,
			CreatedAt:             model.CreatedAt,
		}
		leaderCommits = append(leaderCommits, commitData)
	}

	return leaderCommits, nil
}

func GetRoundsToProcess() ([]*utils.LeaderCommitData, error) {
	// Slice to hold DB model results
	var leaderCommitModels []LeaderCommitScheme

	err := GetDB().Model(&leaderCommitModels).
		Where("random_number_generated = ? AND submit_merkle_root_done = ?", false, true).
		Select()
	if err != nil {
		return nil, err
	}

	// Map DB models to utils.LeaderCommitData slice
	leaderCommits := make([]*utils.LeaderCommitData, 0, len(leaderCommitModels))
	for _, model := range leaderCommitModels {
		sign := &utils.SignInfo{
			R: model.SignR,
			S: model.SignS,
			V: model.SignV,
		}

		commitData := &utils.LeaderCommitData{
			UniqueKey:             model.UniqueKey,
			Round:                 model.Round,
			TrialNum:              model.TrialNum,
			EOAAddress:            model.EOAAddress,
			Cvs:                   utils.ConvertByteArray(model.Cvs),
			CvsHex:                model.CvsHex,
			Cos:                   utils.ConvertByteArray(model.Cos),
			CosHex:                model.CosHex,
			SecretValue:           utils.ConvertByteArray(model.SecretValue),
			SecretValueHex:        model.SecretValueHex,
			Sign:                  *sign,
			SubmitMerkleRootDone:  model.SubmitMerkleRootDone,
			RandomNumberGenerated: model.RandomNumberGenerated,
			CreatedAt:             model.CreatedAt,
		}
		leaderCommits = append(leaderCommits, commitData)
	}

	return leaderCommits, nil
}

func UpdateLeaderCommit(leaderCommit *utils.LeaderCommitData) error {
	// Convert utils.LeaderCommitData to DB model LeaderCommit
	model := LeaderCommitScheme{
		UniqueKey:             leaderCommit.UniqueKey,
		Round:                 leaderCommit.Round,
		TrialNum:              leaderCommit.TrialNum,
		EOAAddress:            leaderCommit.EOAAddress,
		Cvs:                   leaderCommit.Cvs[:], // convert [32]byte to []byte
		CvsHex:                leaderCommit.CvsHex,
		Cos:                   leaderCommit.Cos[:], // convert [32]byte to []byte
		CosHex:                leaderCommit.CosHex,
		SecretValue:           leaderCommit.SecretValue[:], // convert [32]byte to []byte
		SecretValueHex:        leaderCommit.SecretValueHex,
		SignR:                 leaderCommit.Sign.R,
		SignS:                 leaderCommit.Sign.S,
		SignV:                 leaderCommit.Sign.V,
		SubmitMerkleRootDone:  leaderCommit.SubmitMerkleRootDone,
		RandomNumberGenerated: leaderCommit.RandomNumberGenerated,
		CreatedAt:             leaderCommit.CreatedAt,
	}

	_, err := GetDB().Model(&model).Where("round = ? AND eoa_address = ?", leaderCommit.Round, leaderCommit.EOAAddress).Update()
	if err != nil {
		return err
	}

	return nil
}

func UpdateLeaderCommitRandomNumberGenerated(round string, trialNum string) error {
	// Create a model instance with only the updated field set
	leaderCommit := LeaderCommitScheme{
		RandomNumberGenerated: true,
	}

	uniqueKey := utils.GetUniqueKey(round, trialNum)
	// Update only the "random_number_generated" column where uniqueKey matches
	_, err := GetDB().Model(&leaderCommit).
		Column("random_number_generated").
		Where("unique_key = ?", uniqueKey).
		Update()
	if err != nil {
		return err
	}

	return nil
}

func GetRegisteredNodes() ([]*utils.NodeInfo, error) {
	// Query DB model structs
	var registeredNodes []NodeInfoScheme

	err := GetDB().Model(&registeredNodes).Select()
	if err != nil {
		return nil, err
	}

	// Map DB models to utils.NodeInfo
	nodes := make([]*utils.NodeInfo, 0, len(registeredNodes))
	for _, rn := range registeredNodes {
		node := &utils.NodeInfo{
			IP:         rn.IP,
			Port:       rn.Port,
			PeerID:     rn.PeerID,
			EOAAddress: rn.EOAAddress,
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func AddRevealOrder(revealOrder *utils.RevealOrderData) error {
	model := RevealOrderScheme{
		Round:        revealOrder.Round,
		TrialNum:     revealOrder.TrialNum,
		OrderedNodes: revealOrder.OrderedNodes,
		RevealOrder:  revealOrder.RevealOrder,
		RV:           revealOrder.RV,
	}

	_, err := GetDB().Model(&model).Insert()
	return err
}

func GetRevealOrders() ([]*utils.RevealOrderData, error) {
	var models []RevealOrderScheme
	err := GetDB().Model(&models).Select()
	if err != nil {
		return nil, err
	}

	revealOrders := make([]*utils.RevealOrderData, 0, len(models))
	for _, m := range models {
		revealOrders = append(revealOrders, &utils.RevealOrderData{
			Round:        m.Round,
			TrialNum:     m.TrialNum,
			OrderedNodes: m.OrderedNodes,
			RevealOrder:  m.RevealOrder,
			RV:           m.RV,
		})
	}

	return revealOrders, nil
}

func GetRevealOrder(round, trailNum, uniqueKey string) (*utils.RevealOrderData, error) {
	var model RevealOrderScheme
	err := GetDB().Model(&model).
		Where("unique_key = ?", uniqueKey).
		Limit(1).
		Select()
	if err != nil {
		return nil, err
	}

	revealOrder := &utils.RevealOrderData{
		Round:        model.Round,
		TrialNum:     model.TrialNum,
		OrderedNodes: model.OrderedNodes,
		RevealOrder:  model.RevealOrder,
		RV:           model.RV,
	}

	return revealOrder, nil
}

func AddCommit(commit *utils.CommitData) error {

	model := CommitDataScheme{
		UniqueKey:       commit.UniqueKey,
		Round:           commit.Round,
		Cvs:             commit.Cvs[:], // convert [32]byte to []byte
		Cos:             commit.Cos[:],
		SecretValue:     commit.SecretValue[:],
		SignR:           commit.Sign.R,
		SignS:           commit.Sign.S,
		SignV:           commit.Sign.V,
		SendToLeader:    commit.SendToLeader,
		SendCosToLeader: commit.SendCosToLeader,
	}

	_, err := GetDB().Model(&model).Insert()
	return err
}

func UpdateCommit(commit *utils.CommitData) error {
	model := CommitDataScheme{
		UniqueKey:       commit.UniqueKey,
		Round:           commit.Round,
		TrialNum:        commit.TrialNum,
		Cvs:             commit.Cvs[:],
		Cos:             commit.Cos[:],
		SecretValue:     commit.SecretValue[:],
		SignR:           commit.Sign.R,
		SignS:           commit.Sign.S,
		SignV:           commit.Sign.V,
		SendToLeader:    commit.SendToLeader,
		SendCosToLeader: commit.SendCosToLeader,
	}

	_, err := GetDB().Model(&model).Where("round = ?", commit.Round).Update()
	return err
}

func GetCommitByRound(round string, trialNum string) (*utils.CommitData, error) {
	var model CommitDataScheme
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	err := GetDB().Model(&model).Where("unique_key = ?", uniqueKey).Limit(1).Select()
	if err != nil {
		log.Printf("Failed to get commit info: %v", err)
		return nil, err
	}

	// Convert DB model CommitDataModel to domain CommitData
	var cvs, cos, secretValue [32]byte
	copy(cvs[:], model.Cvs)
	copy(cos[:], model.Cos)
	copy(secretValue[:], model.SecretValue)

	commit := &utils.CommitData{
		UniqueKey:   uniqueKey,
		Round:       round,
		TrialNum:    trialNum,
		Cvs:         cvs,
		Cos:         cos,
		SecretValue: secretValue,
		Sign: utils.SignInfo{
			R: model.SignR,
			S: model.SignS,
			V: model.SignV,
		},
		SendToLeader:    model.SendToLeader,
		SendCosToLeader: model.SendCosToLeader,
	}

	return commit, nil
}

func AddBroadcastTracker(trackerData *utils.BroadcastTracker) error {
	tracker := BroadcastTrackerScheme{
		Round:        trackerData.Round,
		TrialNum:     trackerData.TrialNum,
		EOAAddress:   trackerData.EOAAddress,
		Type:         trackerData.Type,
		MessageID:    trackerData.MessageID,
		Data:         trackerData.Data[:],
		Attempts:     trackerData.Attempts,
		MaxAttempts:  trackerData.MaxAttempts,
		Acknowledged: trackerData.Acknowledged,
		LastSent:     trackerData.LastSent,
		Timeout:      trackerData.Timeout,
	}

	_, err := GetDB().Model(&tracker).Insert()
	if err != nil {
		return err
	}

	return nil
}

func GetBroadcastTrackers() ([]*utils.BroadcastTracker, error) {
	var broadcastTrackerModels []BroadcastTrackerScheme

	err := GetDB().Model(&broadcastTrackerModels).Select()
	if err != nil {
		return nil, err
	}

	broadcastTrackers := make([]*utils.BroadcastTracker, 0, len(broadcastTrackerModels))
	for _, model := range broadcastTrackerModels {
		broadcastTracker := &utils.BroadcastTracker{
			Round:        model.Round,
			TrialNum:     model.TrialNum,
			EOAAddress:   model.EOAAddress,
			Type:         model.Type,
			MessageID:    model.MessageID,
			Data:         utils.ConvertByteArray(model.Data),
			Attempts:     model.Attempts,
			MaxAttempts:  model.MaxAttempts,
			Acknowledged: model.Acknowledged,
			LastSent:     model.LastSent,
			Timeout:      model.Timeout,
		}
		broadcastTrackers = append(broadcastTrackers, broadcastTracker)
	}

	return broadcastTrackers, nil
}

func UpdateBroadcastTracker(tracker *utils.BroadcastTracker) error {
	model := BroadcastTrackerScheme{
		Round:        tracker.Round,
		TrialNum:     tracker.TrialNum,
		EOAAddress:   tracker.EOAAddress,
		Type:         tracker.Type,
		MessageID:    tracker.MessageID,
		Data:         tracker.Data[:],
		Attempts:     tracker.Attempts,
		MaxAttempts:  tracker.MaxAttempts,
		Acknowledged: tracker.Acknowledged,
		LastSent:     tracker.LastSent,
		Timeout:      tracker.Timeout,
	}
	_, err := GetDB().Model(&model).Where("round = ? AND trial_num = ? AND eoa_address = ? AND type = ? AND message_id = ?", tracker.Round, tracker.TrialNum, tracker.EOAAddress, tracker.Type, tracker.MessageID).Update()
	return err
}

func AddAllBroadcastTrackers(trackers []*utils.BroadcastTracker) error {
	for _, tracker := range trackers {
		err := AddBroadcastTracker(tracker)
		if err != nil {
			return fmt.Errorf("failed to add broadcast tracker data for %s_%s_%s_%s_%s", tracker.Round, tracker.TrialNum, tracker.EOAAddress, tracker.Type, tracker.MessageID)
		}
	}

	return nil
}

func DeleteBroadcastTracker(tracker *utils.BroadcastTracker) error {
	model := BroadcastTrackerScheme{}
	_, err := GetDB().Model(&model).Where("round = ? AND trial_num = ? AND eoa_address = ? AND type = ? AND message_id = ?", tracker.Round, tracker.TrialNum, tracker.EOAAddress, tracker.Type, tracker.MessageID).Delete()
	return err
}

func AddPeerCommitData(peerData *PeerCommitDataScheme) error {
	peerCommit := PeerCommitDataScheme{
		UniqueKey:   peerData.UniqueKey,
		Round:       peerData.Round,
		TrialNum:    peerData.TrialNum,
		EOAAddress:  peerData.EOAAddress,
		SecretValue: peerData.SecretValue[:],
		Cos:         peerData.Cos[:],
		Cvs:         peerData.Cvs[:],
	}

	_, err := GetDB().Model(&peerCommit).Insert()
	if err != nil {
		return err
	}

	return nil
}

func GetPeerCommitData(round string, trialNum string, eoaAddress string) (*PeerCommitDataScheme, error) {
	var peerCommit PeerCommitDataScheme
	uniqueKey := utils.GetUniqueKey(round, trialNum)
	err := GetDB().Model(&peerCommit).
		Where("unique_key = ? AND eoa_address = ?", uniqueKey, eoaAddress).
		Select()
	if err != nil {
		return nil, err
	}

	return &peerCommit, nil
}

func UpdatePeerCommitData(peerData *PeerCommitDataScheme) error {
	peerCommit := PeerCommitDataScheme{
		UniqueKey:   peerData.UniqueKey,
		Round:       peerData.Round,
		TrialNum:    peerData.TrialNum,
		EOAAddress:  peerData.EOAAddress,
		SecretValue: peerData.SecretValue[:],
		Cos:         peerData.Cos[:],
		Cvs:         peerData.Cvs[:],
	}

	_, err := GetDB().Model(&peerCommit).
		Where("unique_key = ? AND eoa_address = ?", peerCommit.UniqueKey, peerCommit.EOAAddress).
		Update()
	if err != nil {
		return err
	}

	return nil
}
