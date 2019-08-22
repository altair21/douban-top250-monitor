package logger

import (
	"database/sql/driver"
	"fmt"
	"github.com/altair21/douban-top250-monitor/logger/lumberjack"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/robfig/cron"
)

var (
	sqlRegexp                = regexp.MustCompile(`\?`)
	numericPlaceHolderRegexp = regexp.MustCompile(`\$\d+`)
)

func sqlFormat(sqls interface{}, params interface{}) string {
	var (
		sql             string
		formattedValues []string
	)
	for _, value := range params.([]interface{}) {
		indirectValue := reflect.Indirect(reflect.ValueOf(value))
		if indirectValue.IsValid() {
			value = indirectValue.Interface()
			if t, ok := value.(time.Time); ok {
				formattedValues = append(formattedValues, fmt.Sprintf("'%v'", t.Format("2006-01-02 15:04:05")))
			} else if b, ok := value.([]byte); ok {
				if str := string(b); isPrintable(str) {
					formattedValues = append(formattedValues, fmt.Sprintf("'%v'", str))
				} else {
					formattedValues = append(formattedValues, "'<binary>'")
				}
			} else if r, ok := value.(driver.Valuer); ok {
				if value, err := r.Value(); err == nil && value != nil {
					formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
				} else {
					formattedValues = append(formattedValues, "NULL")
				}
			} else {
				formattedValues = append(formattedValues, fmt.Sprintf("'%v'", value))
			}
		} else {
			formattedValues = append(formattedValues, "NULL")
		}
	}

	if numericPlaceHolderRegexp.MatchString(sqls.(string)) {
		sql = sqls.(string)
		for index, value := range formattedValues {
			placeholder := fmt.Sprintf(`\$%d([^\d]|$)`, index+1)
			sql = regexp.MustCompile(placeholder).ReplaceAllString(sql, value+"$1")
		}
	} else {
		formattedValuesLength := len(formattedValues)
		for index, value := range sqlRegexp.Split(sqls.(string), -1) {
			sql += value
			if index < formattedValuesLength {
				sql += formattedValues[index]
			}
		}
	}

	return sql
}

func isPrintable(s string) bool {
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func scheduleRotate(l *lumberjack.Logger, lDB *lumberjack.Logger) {
	c := cron.New()
	c.AddFunc("0 0 0 * * *", func() {
		l.Rotate()
		lDB.Rotate()
	})
	c.Start()
}

// DBLogWrapper model
type DBLogWrapper struct{}

// NewDBLog constructor
func NewDBLog() *DBLogWrapper {
	return &DBLogWrapper{}
}

// Print is overwrite gorm logger
func (l *DBLogWrapper) Print(v ...interface{}) {
	if len(v) < 6 {
		dbLogger.Errorf("%s", v)
	} else {
		use := v[2]
		sql := v[3]
		params := v[4]
		formatSQL := strings.Replace(sqlFormat(sql, params), "\"", "'", -1)
		dbLogger.Debugf("%s [%s]", formatSQL, use)
	}
}

// MyLogger model
type MyLogger struct{}

// NewMyLogger constructor
func NewMyLogger() *MyLogger {
	return &MyLogger{}
}

// Debugf format
func (l *MyLogger) Debugf(format string, args ...interface{}) {
	formatStr := strings.Replace(fmt.Sprintf(format, args...), "\"", "'", -1)
	logger.Debug(formatStr)
}

// Debug format
func (l *MyLogger) Debug(args ...interface{}) {
	formatStr := strings.Replace(fmt.Sprintf("%v", args...), "\"", "'", -1)
	logger.Debug(formatStr)
}

func (l *MyLogger) Error(args ...interface{}) {
	formatStr := strings.Replace(fmt.Sprintf("%v", args...), "\"", "'", -1)
	logger.Error(formatStr)
}

// Errorf format
func (l *MyLogger) Errorf(format string, args ...interface{}) {
	formatStr := strings.Replace(fmt.Sprintf(format, args...), "\"", "'", -1)
	logger.Error(formatStr)
}

// Info format
func (l *MyLogger) Info(args ...interface{}) {
	formatStr := strings.Replace(fmt.Sprintf("%v", args...), "\"", "'", -1)
	logger.Info(formatStr)
}

// Infof format
func (l *MyLogger) Infof(format string, args ...interface{}) {
	formatStr := strings.Replace(fmt.Sprintf(format, args...), "\"", "'", -1)
	logger.Info(formatStr)
}
