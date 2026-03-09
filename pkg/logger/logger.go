package logger

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func Init() {
	output := zerolog.ConsoleWriter{Out: os.Stdout, NoColor: false}
	log.Logger = zerolog.New(output).With().Timestamp().Logger()
}
