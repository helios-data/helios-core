package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	ansiReset   = "\x1b[0m"
	ansiDim     = "\x1b[90m"
	ansiRed     = "\x1b[31m"
	ansiYellow  = "\x1b[33m"
	ansiBlue    = "\x1b[34m"
	ansiMagenta = "\x1b[35m"
)

// Console encoders must append a single string so each logical segment stays one column.
func bracketedTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	s := ansiDim + "[" + t.Format("15:04:05.000") + "]" + ansiReset
	enc.AppendString(s)
}

func bracketedCapitalColorLevelEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	var c string
	switch l {
	case zapcore.DebugLevel:
		c = ansiMagenta
	case zapcore.InfoLevel:
		c = ansiBlue
	case zapcore.WarnLevel:
		c = ansiYellow
	case zapcore.ErrorLevel, zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel:
		c = ansiRed
	default:
		c = ansiRed
	}
	enc.AppendString("[" + c + l.CapitalString() + ansiReset + "]")
}

func developmentBracketEncoderConfig() zapcore.EncoderConfig {
	cfg := zap.NewDevelopmentEncoderConfig()
	cfg.EncodeTime = bracketedTimeEncoder
	cfg.EncodeLevel = bracketedCapitalColorLevelEncoder
	cfg.ConsoleSeparator = " "
	cfg.CallerKey = ""
	cfg.NameKey = ""
	return cfg
}

// ParseLevel maps CLI names to zap levels. "verbose" is an alias for "debug".
func ParseLevel(s string) (zapcore.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "error":
		return zapcore.ErrorLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "debug", "verbose":
		return zapcore.DebugLevel, nil
	default:
		var z zapcore.Level
		return z, fmt.Errorf("unknown log level %q (want error, warn, info, debug, verbose)", s)
	}
}

// Init builds a development-style console logger, sets it as zap's global logger, and writes to w.
func Init(level string, w io.Writer) error {
	lvl, err := ParseLevel(level)
	if err != nil {
		return err
	}
	encCfg := developmentBracketEncoderConfig()
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encCfg),
		zapcore.AddSync(w),
		zap.NewAtomicLevelAt(lvl),
	)
	log := zap.New(core, zap.Development(), zap.AddStacktrace(zapcore.ErrorLevel))
	zap.ReplaceGlobals(log)
	return nil
}

// MustInit is like Init but prints to stderr and exits on bad level.
func MustInit(level string, w io.Writer) {
	if err := Init(level, w); err != nil {
		fmt.Fprintf(os.Stderr, "helios: %v\n", err)
		os.Exit(2)
	}
}

// L returns the global [*zap.Logger] for advanced use (e.g. custom cores, hooks).
func L() *zap.Logger { return zap.L() }

// Sync flushes any buffered log entries; call on shutdown (ignore errors on stderr).
func Sync() error { return zap.L().Sync() }

// With returns a child logger that always includes the given fields (use [zap.String], etc.).
func With(fields ...zap.Field) *zap.Logger { return zap.L().With(fields...) }

func Debug(msg string, fields ...zap.Field) { zap.L().Debug(msg, fields...) }
func Info(msg string, fields ...zap.Field)  { zap.L().Info(msg, fields...) }
func Warn(msg string, fields ...zap.Field)  { zap.L().Warn(msg, fields...) }
func Error(msg string, fields ...zap.Field) { zap.L().Error(msg, fields...) }

func Debugw(msg string, keysAndValues ...interface{}) { zap.S().Debugw(msg, keysAndValues...) }
func Infow(msg string, keysAndValues ...interface{})  { zap.S().Infow(msg, keysAndValues...) }
func Warnw(msg string, keysAndValues ...interface{})  { zap.S().Warnw(msg, keysAndValues...) }
func Errorw(msg string, keysAndValues ...interface{}) { zap.S().Errorw(msg, keysAndValues...) }

func Fatal(msg string, fields ...zap.Field) { zap.L().Fatal(msg, fields...) }
func Panic(msg string, fields ...zap.Field) { zap.L().Panic(msg, fields...) }
