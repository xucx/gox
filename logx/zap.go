package logx

import (
	"fmt"
	"io"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type ZapLogger struct {
	s *zap.SugaredLogger
}

func (z *ZapLogger) Log(level Level, args ...any) {
	switch level {
	case LevelDebug:
		z.s.Debug(args...)
	case LevelInfo:
		z.s.Info(args...)
	case LevelWarn:
		z.s.Warn(args...)
	case LevelError:
		z.s.Error(args...)
	case LevelDPanic:
		z.s.DPanic(args...)
	case LevelPanic:
		z.s.Panic(args...)
	case LevelFatal:
		z.s.Fatal(args...)
	}
}

func (z *ZapLogger) Logf(level Level, format string, args ...any) {
	switch level {
	case LevelDebug:
		z.s.Debugf(format, args...)
	case LevelInfo:
		z.s.Infof(format, args...)
	case LevelWarn:
		z.s.Warnf(format, args...)
	case LevelError:
		z.s.Errorf(format, args...)
	case LevelDPanic:
		z.s.DPanicf(format, args...)
	case LevelPanic:
		z.s.Panicf(format, args...)
	case LevelFatal:
		z.s.Fatalf(format, args...)
	}
}

func (z *ZapLogger) Logw(level Level, msg string, kvs ...any) {
	switch level {
	case LevelDebug:
		z.s.Debugw(msg, kvs...)
	case LevelInfo:
		z.s.Infow(msg, kvs...)
	case LevelWarn:
		z.s.Warnw(msg, kvs...)
	case LevelError:
		z.s.Errorw(msg, kvs...)
	case LevelDPanic:
		z.s.DPanicw(msg, kvs...)
	case LevelPanic:
		z.s.Panicw(msg, kvs...)
	case LevelFatal:
		z.s.Fatalw(msg, kvs...)
	}
}

func WrapZapLogger(l *zap.Logger, name ...string) Logger {
	if len(name) > 0 {
		l = l.Named(strings.Join(name, "."))
	}
	return &ZapLogger{s: l.Sugar()}
}

func NewZapLogger(conf Config) Logger {
	logLevel, _ := zapLevelFromString(conf.Level)
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	var (
		syncer  zapcore.WriteSyncer
		encoder zapcore.Encoder
	)

	if conf.File != "" {
		maxSize := conf.FileMaxAge
		if maxSize <= 0 {
			maxSize = 10
		}

		maxBackups := conf.FileMaxBackups
		if maxBackups <= 0 {
			maxBackups = 3
		}

		maxAge := conf.FileMaxAge
		if maxAge <= 0 {
			maxAge = 30
		}

		syncer = newZapNoSync(&lumberjack.Logger{
			Filename:   conf.File,  // Location of the log file
			MaxSize:    maxSize,    // Maximum file size (in MB)
			MaxBackups: maxBackups, // Maximum number of old files to retain
			MaxAge:     maxAge,     // Maximum number of days to retain old files
			Compress:   true,       // Whether to compress/archive old files
			LocalTime:  true,       // Use local time for timestamps
		})
	} else {
		syncer = os.Stderr
	}

	if conf.Format == FormatJson {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	writer := zapcore.Lock(syncer)

	return WrapZapLogger(
		zap.New(
			zapcore.NewCore(
				encoder,
				writer,
				logLevel,
			),
		),
	)
}

func zapLevelFromString(levelStr string) (zapcore.Level, error) {
	level := zapcore.InfoLevel
	if err := level.Set(levelStr); err != nil {
		return zapcore.InfoLevel, fmt.Errorf("log level %w parse fail", err)
	}

	return level, nil
}

type zapNoSync struct {
	io.Writer
}

func (zapNoSync) Sync() error {
	return nil
}

func newZapNoSync(w io.Writer) zapcore.WriteSyncer {
	return zapNoSync{
		Writer: w,
	}
}
