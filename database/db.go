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
	_, err := GetDB().Model(nodeInfo).Insert()
	if err != nil {
		return err
	}

	return nil
}

func UpdateNodeInfo(nodeInfo *utils.NodeInfo) error {
	_, err := GetDB().Model(nodeInfo).WherePK().Insert()
	if err != nil {
		return err
	}

	return nil
}

func GetNodeInfo() (*utils.NodeInfo, error) {
	var nodeInfo *utils.NodeInfo
	err := GetDB().Model(nodeInfo).Select()
	if err != nil {
		return nil, err
	}

	return nodeInfo, nil
}

func AddLeaderCommit(leaderCommits *utils.LeaderCommitData) error {
	// If Cvs is present, also store the hex value
	if leaderCommits.Cvs != [32]byte{} {
		leaderCommits.CvsHex = hex.EncodeToString(leaderCommits.Cvs[:]) // Convert Cvs byte array to hex string
	}
	_, err := GetDB().Model(leaderCommits).Insert()
	if err != nil {
		return err
	}

	return nil
}

func GetLeaderCommitByRoundAndEoaAddr(round string, eoaAddr string) (*utils.LeaderCommitData, error) {
	var leaderCommit utils.LeaderCommitData
	err := GetDB().Model(leaderCommit).Where("round = ? AND eoa_address = ?", round, eoaAddr).Select()
	if err != nil {
		return nil, err
	}

	return &leaderCommit, nil
}

func GetLeaderCommitsByRound(round string) ([]*utils.LeaderCommitData, error) {
	var leaderCommits []*utils.LeaderCommitData
	err := GetDB().Model(&leaderCommits).Where("round = ?", round).Select()
	if err != nil {
		return nil, err
	}

	return leaderCommits, nil
}

func GetRoundsToProcess() ([]*utils.LeaderCommitData, error) {
	var leaderCommits []*utils.LeaderCommitData
	err := GetDB().Model(leaderCommits).Where("random_number_generated = ? AND submit_merkle_root_done = ?", false, true).Select()
	if err != nil {
		return nil, err
	}

	return leaderCommits, nil
}

func UpdateLeaderCommit(leaderCommit *utils.LeaderCommitData) error {
	_, err := GetDB().Model(leaderCommit).WherePK().Update()
	if err != nil {
		return err
	}

	return nil
}

func UpdateLeaderCommitRandomNumberGenerated(round string) error {
	leaderCommitSchema := utils.LeaderCommitData{
		RandomNumberGenerated: true,
	}
	_, err := GetDB().Model(&leaderCommitSchema).Column("random_number_generated").Update()
	if err != nil {
		return err
	}

	return nil
}

func GetRegisteredNodes() ([]*utils.NodeInfo, error) {
	var nodes []*utils.NodeInfo
	err := GetDB().Model(nodes).Select()
	if err != nil {
		return nil, err
	}

	return nodes, nil
}

func AddRevealOrder(revealOrder *utils.RevealOrderData) error {
	_, err := GetDB().Model(revealOrder).Insert()
	if err != nil {
		return err
	}

	return nil
}

func GetRevealOrders() ([]*utils.RevealOrderData, error) {
	var revealOrders []*utils.RevealOrderData
	err := GetDB().Model(revealOrders).Select()
	if err != nil {
		return nil, err
	}

	return revealOrders, nil
}

func GetRevealOrder(round string) (*utils.RevealOrderData, error) {
	var revealOrder utils.RevealOrderData
	err := GetDB().Model(revealOrder).Where("round = ?", round).Select()
	if err != nil {
		return nil, err
	}

	return &revealOrder, nil
}

func AddCommit(commit *utils.CommitData) error {
	_, err := GetDB().Model(commit).Insert()
	if err != nil {
		return err
	}

	return nil
}

func GetCommitByRound(round string) (*utils.CommitData, error) {
	var commit *utils.CommitData
	err := GetDB().Model(&commit).Where("round = ?", round).Select()
	if err != nil {
		return nil, err
	}

	return commit, nil
}
