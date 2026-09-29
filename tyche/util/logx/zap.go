package logx

import (
	"os"

	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Config struct {
	// Filemame is the file to write logs to.  Backup log files will be retained
	// in the same directory.  It uses <processname>-lumberjack.log in
	// os.TempDir() if empty.
	Filemame string `json:"filename" yaml:"filename" toml:"filename"`

	// A Level is a logging priority. Higher levels are more important.
	Level string `json:"level" yaml:"level" toml:"level"`

	// Named adds a sub-scope to the logger's name. See Logger.Named for details.
	Named string `json:"named" yaml:"named" toml:"named"`

	// MaxSize is the maximum size in megabytes of the log file before it gets
	// rotated. It defaults to 100 megabytes.
	MaxSize int `json:"maxsize" yaml:"maxsize" toml:"maxsize"`

	// MaxAge is the maximum number of days to retain old log files based on the
	// timestamp encoded in their filename.  Note that a day is defined as 24
	// hours and may not exactly correspond to calendar days due to daylight
	// savings, leap seconds, etc. The default is not to remove old log files
	// based on age.
	MaxAge int `json:"maxage" yaml:"maxage" toml:"maxage"`

	// MaxBackups is the maximum number of old log files to retain.  The default
	// is to retain all old log files (though MaxAge may still cause them to get
	// deleted.)
	MaxBackups int `json:"maxbackups" yaml:"maxbackups" toml:"maxbackups"`

	// LocalTime determines if the time used for formatting the timestamps in
	// backup files is the computer's local time.  The default is to use UTC
	// time.
	LocalTime bool `json:"localtime" yaml:"localtime" toml:"localtime"`

	// Compress determines if the rotated log files should be compressed
	// using gzip. The default is not to perform compression.
	Compress bool `json:"compress" yaml:"compress" toml:"compress"`
}

func Default(name string) *zap.SugaredLogger {
	return New(&Config{
		Named:     name,
		Level:     "debug",
		LocalTime: true,
	})
}

func New(conf *Config) *zap.SugaredLogger {
	var encoderConf = getEncoderConfig()
	var writeSyncer = getWriteSyncer(conf)
	var encoder = zapcore.NewConsoleEncoder(encoderConf)
	var level = parseLevel(conf.Level)
	var core = zapcore.NewCore(encoder, writeSyncer, level)
	var addCaller = zap.AddCaller()

	return zap.New(core, addCaller).Named(conf.Named).Sugar()
}

func getEncoderConfig() zapcore.EncoderConfig {
	var conf = zap.NewDevelopmentEncoderConfig()
	conf.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")
	return conf
}

func getWriteSyncer(conf *Config) zapcore.WriteSyncer {
	if conf.Filemame == "" {
		return os.Stdout
	}

	var writeSyncer = zapcore.AddSync(&lumberjack.Logger{
		Filename:   conf.Filemame,
		Compress:   conf.Compress,
		MaxAge:     conf.MaxAge,
		MaxBackups: conf.MaxBackups,
		MaxSize:    conf.MaxSize,
		LocalTime:  conf.LocalTime,
	})
	return zapcore.NewMultiWriteSyncer(os.Stdout, writeSyncer)
}

func parseLevel(str string) zapcore.LevelEnabler {
	var level, err = zapcore.ParseLevel(str)
	if err != nil {
		return zapcore.InfoLevel
	}
	return level
}
