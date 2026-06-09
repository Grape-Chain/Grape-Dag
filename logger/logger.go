package logger

import (
	"fmt"
	"os"

	"github.com/Grape-Chain/Grape-Dag/config"
	"github.com/Grape-Chain/Grape-Dag/utils"
	golog "github.com/ipfs/go-log/v2"
)

func InitLogging(logFilePre string, console bool, level golog.LogLevel) {
	if console && level < golog.LevelFatal {
		tmpdir := os.TempDir()
		lf, err := os.CreateTemp(tmpdir, fmt.Sprintf(config.LOG_FILE_PREFIX, logFilePre))
		if err != nil {
			utils.ColorizePrint("Failed to create a temp log file. [Out of space?]", err)
			return
		}
		defer lf.Close()
		os.Setenv(config.GOLOG_FILE, lf.Name())
		output_mode := config.GOLOG_DEFAULT_OUTPUT_MODE
		if console {
			output_mode += "+stdout"
		}
		os.Setenv(config.GOLOG_OUTPUT, output_mode)

		cfg := golog.GetConfig()
		cfg.File = lf.Name()
		cfg.Level = level
		cfg.Stderr = false
		cfg.Stdout = console
		golog.SetupLogging(cfg)
		utils.ColorizePrint("\n[LOG] Writing to log %s\n\n", lf.Name())
	}
}
