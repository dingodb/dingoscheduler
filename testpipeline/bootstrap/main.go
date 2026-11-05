// Command bootstrap prepares ONLY an empty, isolated test-pipeline database.
// It is not a product migration or a production administration command.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"

	"dingoscheduler/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const schemaVersion = "scheduler-legacy-empty-v1"

func main() {
	certDir := flag.String("cert-dir", "", "generate an isolated test CA and mutual TLS certificates")
	database := flag.String("database", "", "empty pipeline_* database")
	passwordFile := flag.String("password-file", "", "database password file")
	flag.Parse()
	var err error
	if *certDir != "" {
		err = certificates(*certDir)
	} else {
		err = initialize(*database, *passwordFile)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func initialize(database, passwordFile string) error {
	if !regexp.MustCompile(`^pipeline_[a-z0-9_]{1,48}$`).MatchString(database) {
		return fmt.Errorf("refusing database outside isolated pipeline namespace")
	}
	password, err := os.ReadFile(passwordFile)
	if err != nil {
		return err
	}
	// Fixed Docker alias, dedicated account and database. Never accepts an arbitrary DSN.
	dsn := fmt.Sprintf("pipeline:%s@tcp(mysql:3306)/%s?charset=utf8mb4&parseTime=true&timeout=5s", strings.TrimSpace(string(password)), database)
	// Historical schemas use second-precision CURRENT_TIMESTAMP defaults,
	// matching pkg/gorm/dingo_integration_test.go's legacy baseline.
	zeroPrecision := 0
	db, err := gorm.Open(mysql.New(mysql.Config{DSN: dsn, DefaultDatetimePrecision: &zeroPrecision}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fmt.Errorf("isolated database connection failed")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	var tables int64
	if err = db.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&tables).Error; err != nil {
		return err
	}
	if tables != 0 {
		return fmt.Errorf("refusing bootstrap: database has %d existing tables", tables)
	}
	// These are the legacy tables not migrated by the formal Scheduler main.
	// Use the selected checkout's model definitions; its source hash is recorded by the runner.
	models := []any{&model.Dingospeed{}, &model.CacheJob{}, &model.ModelFileRecord{}, &model.ModelFileProcess{}, &model.Repository{}, &model.RepositoryTag{}, &model.Tag{}, &model.Organization{}, &model.HfToken{}}
	if err = db.AutoMigrate(models...); err != nil {
		return fmt.Errorf("legacy schema initialization: %w", err)
	}
	// No remote credentials or business records are seeded. Registration must create the node.
	var names []string
	if err = db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY table_name").Scan(&names).Error; err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"schemaVersion": schemaVersion, "tables": names, "seededBusinessRows": 0})
}

func certificates(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for _, name := range []string{"ca.crt", "ca.key", "server.crt", "server.key", "client.crt", "client.key"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			return fmt.Errorf("refusing to overwrite certificate material")
		}
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ephemeral-test-pipeline-ca"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(48 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		return err
	}
	if err = writePair(dir, "ca", der, key); err != nil {
		return err
	}
	for i, name := range []string{"server", "client"} {
		leafKey, e := rsa.GenerateKey(rand.Reader, 2048)
		if e != nil {
			return e
		}
		leaf := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 2)), Subject: pkix.Name{CommonName: "test-pipeline-" + name}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, DNSNames: []string{"scheduler", "zetyun.com"}}
		leafDER, e := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
		if e != nil {
			return e
		}
		if err = writePair(dir, name, leafDER, leafKey); err != nil {
			return err
		}
	}
	return nil
}

func writePair(dir, name string, cert []byte, key *rsa.PrivateKey) error {
	if err := os.WriteFile(filepath.Join(dir, name+".crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert}), 0600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".key"), pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600)
}
