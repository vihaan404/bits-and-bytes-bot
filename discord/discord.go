package discord

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/devanbenz/bits-and-bytes-bot/models"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func StartDiscordBot(botSession *discordgo.Session) error {
	if err := botSession.Open(); err != nil {
		slog.Error("error opening websocket connection", err)
		return fmt.Errorf("error opening websocket connection %s", err)
	}
	defer func(discord *discordgo.Session) {
		err := discord.Close()
		if err != nil {
			slog.Error("error closing botSession api", err)
		}
	}(botSession)

	slog.Info("botSession bot generating commands...")
	for _, v := range Commands {
		_, err := botSession.ApplicationCommandCreate(botSession.State.User.ID, "1214625037244432465", v)
		if err != nil {
			slog.Error("Cannot creating command", "name", v.Name, "error", err)
			return fmt.Errorf("error generating commands %s", err)
		}
	}

	return nil
}

func AddDiscordHandlers(botSession *discordgo.Session, state *models.State) {
	botSession.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
	})
	botSession.AddHandler(MessageCreate)
	botSession.Identify.Intents = discordgo.IntentsGuildMessages
	botSession.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		fmt.Println(fmt.Sprintf("dates: %s", state.Dates))
		fmt.Println(fmt.Sprintf("emails: %s", state.Emails))

		if i.Type == discordgo.InteractionMessageComponent && i.MessageComponentData().CustomID == "vote_meeting_time" {
			InsertEmailForVoting(s, i)
		} else if i.Type == discordgo.InteractionModalSubmit {
			email := i.ModalSubmitData().Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
			state.Emails = append(state.Emails, email)

			slog.Info("email added to responses", "email", email)

			VoteForMeeting(s, i, state)
		} else if i.Type == discordgo.InteractionMessageComponent && i.MessageComponentData().CustomID == "date_selection" {
			date := i.MessageComponentData().Values[0]
			state.Votes[date] = state.Votes[date] + 1

			slog.Info("vote cast for next meeting", "date", date, "voteCount", state.Votes[date])

			CompleteVoting(s, i)
		} else if h, ok := CommandHandlers[i.ApplicationCommandData().Name]; ok {
			state.PollDuration = time.Duration(i.ApplicationCommandData().Options[2].IntValue()) * time.Second
			state.Dates = strings.Split(i.ApplicationCommandData().Options[1].StringValue(), ",")
			state.Votes = make(map[string]int, len(state.Dates))
			slog.Info("poll created with duration", "duration", state.PollDuration)

			for _, v := range state.Dates {
				state.Votes[v] = 0
			}

			if state.PollDuration > 0 {
				slog.Info("sending poll timer start channel", "duration", state.PollDuration)
				state.StartPollTimer <- true
			}

			h(s, i)
		}
	})
}

func PollFinished(state *models.State) {
	go func() {
		for {
			enabled := <-state.StartPollTimer
			if enabled && state.PollDuration > 0 {
				slog.Info("poll timer has been enabled", "enabled", state.StartPollTimer, "duration", state.PollDuration)
				time.AfterFunc(state.PollDuration, func() {
					slog.Info("sending payload", "emails", state.Emails, "dates", state.Dates, "votes", state.Votes)
					// TODO: Send data to data store and calendly API
					// TODO: Create a botSession event when poll finishes
					fmt.Println(fmt.Sprintf("emails: %s", state.Emails))
					fmt.Println(fmt.Sprintf("dates: %s", state.Dates))
					fmt.Println(fmt.Sprintf("votes: %s", state.Votes))

					models.ResetState(state)
				})
			}
		}
	}()
}

func TerminateOnSignal() {
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}
