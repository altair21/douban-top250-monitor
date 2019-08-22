package logger

import (
	"os"
	"path"

	"github.com/altair21/douban-top250-monitor/logger/lumberjack"

	"github.com/op/go-logging"
)

var (
	logger   = logging.MustGetLogger("log")
	dbLogger = logging.MustGetLogger("dblog")

	format = logging.MustStringFormatter(
		`[%{time:15:04:05.000}] %{color}%{level:.8s} %{id:08d} %{shortfunc} ▶%{color:reset} %{message}`,
	)

	jsonformat = logging.MustStringFormatter(
		`{"level":"%{level}","msg":"%{message}","time":"%{time:2006-01-02 15:04:05}"}`,
	)

	maxSize = 0
)

// InitializeLogger is initialize func
func InitializeLogger(logDir, serverName, dbName string) {
	logServerPath := path.Join(logDir, serverName+".log")
	l := &lumberjack.Logger{
		Filename:  logServerPath,
		MaxSize:   maxSize,
		LocalTime: true,
	}

	logDBPath := path.Join(logDir, dbName+".log")
	lDB := &lumberjack.Logger{
		Filename:  logDBPath,
		MaxSize:   maxSize,
		LocalTime: true,
	}
	scheduleRotate(l, lDB)

	backendTeminal := logging.NewLogBackend(os.Stdout, "", 0)
	backendFile := logging.NewLogBackend(l, "", 0)
	backendFileDB := logging.NewLogBackend(lDB, "", 0)

	terminalFormatter := logging.NewBackendFormatter(backendTeminal, format)
	fileFormatter := logging.NewBackendFormatter(backendFile, jsonformat)
	fileFormatterDB := logging.NewBackendFormatter(backendFileDB, jsonformat)

	terminalLeveled := logging.AddModuleLevel(terminalFormatter)
	terminalLeveled.SetLevel(logging.DEBUG, "")

	fileLeveled := logging.AddModuleLevel(fileFormatter)
	fileLeveled.SetLevel(logging.DEBUG, "")

	fileLeveledDB := logging.AddModuleLevel(fileFormatterDB)
	fileLeveledDB.SetLevel(logging.DEBUG, "")

	lb := logging.MultiLogger(fileLeveled, terminalLeveled)
	lbDB := logging.MultiLogger(terminalLeveled, fileFormatterDB)
	logger.SetBackend(lb)
	dbLogger.SetBackend(lbDB)
}
