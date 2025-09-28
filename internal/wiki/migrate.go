package wiki

import (
	"context"

	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Migrate applies the wiki schema using Gorm's AutoMigrate and logs progress.
func Migrate(ctx context.Context, db *gorm.DB, logger *logrus.Logger) error {
	if db == nil {
		return eris.New("gorm DB is required")
	}

	logFields := logrus.Fields{"component": "wiki.migrate"}
	if logger != nil {
		logger.WithFields(logFields).Info("applying wiki schema")
	}

	if err := db.WithContext(ctx).AutoMigrate(&Page{}); err != nil {
		if logger != nil {
			logger.WithFields(logFields).WithField("error", err.Error()).Error("wiki schema migration failed")
		}
		return eris.Wrap(err, "auto migrating wiki schema")
	}

	if logger != nil {
		logger.WithFields(logFields).Info("wiki schema migration complete")
	}

	return nil
}
