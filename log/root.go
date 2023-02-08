package log

import (
	"fmt"
	"os"
)

var (
	root          = &logger{[]interface{}{}, new(swapHandler)}
	StdoutHandler = StreamHandler(os.Stdout, LogfmtFormat())
	StderrHandler = StreamHandler(os.Stderr, LogfmtFormat())
)

func init() {
	root.SetHandler(DiscardHandler())
}

func New(ctx ...interface{}) Logger {
	return root.New(ctx...)
}

func Root() Logger {
	return root
}

func Trace(msg string, ctx ...interface{}) {
	root.write(msg, LvlTrace, ctx, skipLevel)
}

func Debug(msg string, ctx ...interface{}) {
	root.write(msg, LvlDebug, ctx, skipLevel)
}

func Info(msg string, ctx ...interface{}) {
	root.write(msg, LvlInfo, ctx, skipLevel)
}

func Warn(msg string, ctx ...interface{}) {
	root.write(msg, LvlWarn, ctx, skipLevel)
}

func Error(msg string, ctx ...interface{}) {
	root.write(msg, LvlError, ctx, skipLevel)
}

func Crit(msg string, ctx ...interface{}) {
	root.write(msg, LvlCrit, ctx, skipLevel)
	os.Exit(1)
}

func Output(msg string, lvl Lvl, calldepth int, ctx ...interface{}) {
	root.write(msg, lvl, ctx, calldepth+skipLevel)
}

func Debugf(format string, args ...interface{}) {
	root.write(fmt.Sprintf(format, args...), LvlDebug, nil, skipLevel)
}

func Infof(format string, args ...interface{}) {
	root.write(fmt.Sprintf(format, args...), LvlInfo, nil, skipLevel)
}

func Warnf(format string, args ...interface{}) {
	root.write(fmt.Sprintf(format, args...), LvlWarn, nil, skipLevel)
}

func Errorf(format string, args ...interface{}) {
	root.write(fmt.Sprintf(format, args...), LvlError, nil, skipLevel)
}
