package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

// Global cache to prevent duplicate thread processing within a short window
var (
	processedThreads = make(map[string]time.Time)
	cacheMutex       sync.Mutex
)

func main() {
	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file, using system environment variables")
	}

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("DISCORD_TOKEN not set")
	}

	// Create a new Discord session using your bot token
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("Error creating Discord session,", err)
		return
	}

	// Add handlers
	dg.AddHandler(messageCreate)
	dg.AddHandler(threadCreate)
	dg.AddHandler(interactionCreate)
	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Println("Bot is ready! Registering commands...")
		commands := []*discordgo.ApplicationCommand{
			{
				Name:        "search",
				Description: "Search for media and create a new forum post",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionString,
						Name:        "query",
						Description: "The title of the movie, anime, or show",
						Required:    true,
					},
				},
			},
		}

		guildID := os.Getenv("GUILD_ID")

		// Register commands to the guild if specified, otherwise globally
		_, err := s.ApplicationCommandBulkOverwrite(s.State.User.ID, guildID, commands)
		if err != nil {
			log.Printf("Cannot register commands: %v\n", err)
		}

		// If a GUILD_ID is set, clear global commands to prevent duplicates from previous runs
		if guildID != "" {
			_, err = s.ApplicationCommandBulkOverwrite(s.State.User.ID, "", nil)
			if err != nil {
				log.Printf("Cannot clear global commands: %v\n", err)
			}
		}
	})

	// Set intents
	dg.Identify.Intents = discordgo.IntentGuildMessages | discordgo.IntentGuilds

	// Open a websocket connection
	err = dg.Open()
	if err != nil {
		fmt.Println("Error opening connection,", err)
		return
	}

	fmt.Println("Bot is now running. Press CTRL+C to exit.")

	// Wait here until CTRL+C is pressed
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	// Cleanly close the session
	dg.Close()
}

// messageCreate handled basic commands
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if m.Content == "!ping" {
		s.ChannelMessageSend(m.ChannelID, "Pong!")
	}
}

// threadCreate handles new forum posts
func threadCreate(s *discordgo.Session, t *discordgo.ThreadCreate) {
	forumChannelID := os.Getenv("FORUM_CHANNEL_ID")
	if forumChannelID == "" || t.ParentID != forumChannelID {
		return
	}

	// Ignore threads created by the bot itself to prevent recursive triggers
	if t.OwnerID == s.State.User.ID {
		return
	}

	// Duplicate prevention cache check
	cacheMutex.Lock()
	if lastProcessed, exists := processedThreads[t.ID]; exists && time.Since(lastProcessed) < 5*time.Second {
		cacheMutex.Unlock()
		return
	}
	processedThreads[t.ID] = time.Now()
	cacheMutex.Unlock()

	title := t.Name
	log.Printf("New forum post detected: %s\n", title)

	// Get results
	options, topEmbed, err := searchAndPreview(title)
	if err != nil || len(options) == 0 {
		return
	}

	// Send selection message with preview
	s.ChannelMessageSendComplex(t.ID, &discordgo.MessageSend{
		Content: "Is this the correct media?",
		Embeds:  []*discordgo.MessageEmbed{topEmbed},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.SelectMenu{
						CustomID:    "media_select",
						Placeholder: "Choose a different one...",
						Options:     options,
					},
				},
			},
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    "Confirm",
						Style:    discordgo.SuccessButton,
						CustomID: "confirm_selection",
					},
					discordgo.Button{
						Label:    "Cancel",
						Style:    discordgo.DangerButton,
						CustomID: "cancel_search",
					},
				},
			},
		},
	})
}

// searchAndPreview performs search on AniList and TMDB and returns options plus top embed
func searchAndPreview(query string) ([]discordgo.SelectMenuOption, *discordgo.MessageEmbed, error) {
	// Combine and score results
	type scoredResult struct {
		option discordgo.SelectMenuOption
		score  int
	}
	var scoredResults []scoredResult

	// 1. Search AniList
	aniResults, err := searchAniList(query)
	if err == nil {
		for _, res := range aniResults {
			label := res.Title.English
			if label == "" {
				label = res.Title.Romaji
			}

			score := calculateScore(query, label)
			scoredResults = append(scoredResults, scoredResult{
				score: score,
				option: discordgo.SelectMenuOption{
					Label:       fmt.Sprintf("[Anime] %s", truncate(label, 90)),
					Value:       fmt.Sprintf("anilist:%d", res.ID),
					Description: fmt.Sprintf("Year: %d | %s", res.SeasonYear, truncate(strings.Join(res.Genres, ", "), 50)),
				},
			})
		}
	}

	// 2. Search TMDB
	tmdbResults, err := searchTMDB(query)
	if err == nil {
		for _, res := range tmdbResults {
			resTitle := res.Title
			if resTitle == "" {
				resTitle = res.Name
			}
			year := res.ReleaseDate
			if year == "" {
				year = res.FirstAirDate
			}
			if len(year) > 4 {
				year = year[:4]
			}

			score := calculateScore(query, resTitle)
			scoredResults = append(scoredResults, scoredResult{
				score: score,
				option: discordgo.SelectMenuOption{
					Label:       fmt.Sprintf("[%s] %s", strings.Title(res.MediaType), truncate(resTitle, 90)),
					Value:       fmt.Sprintf("tmdb_%s:%d", res.MediaType, res.ID),
					Description: fmt.Sprintf("Year: %s", year),
				},
			})
		}
	}

	if len(scoredResults) == 0 {
		return nil, nil, fmt.Errorf("no results found")
	}

	// Sort by score descending
	sort.Slice(scoredResults, func(i, j int) bool {
		return scoredResults[i].score > scoredResults[j].score
	})

	// Build final options (limit to top 25, which is Discord's max)
	var options []discordgo.SelectMenuOption
	for i := 0; i < len(scoredResults) && i < 25; i++ {
		options = append(options, scoredResults[i].option)
	}

	// 3. Fetch full details for the TOP result for the initial preview
	topEmbed, err := getEmbedForValue(scoredResults[0].option.Value)
	if err != nil {
		return nil, nil, err
	}

	return options, topEmbed, nil
}

func getEmbedForValue(value string) (*discordgo.MessageEmbed, error) {
	parts := strings.Split(value, ":")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid value format")
	}
	service := parts[0]
	id, _ := strconv.Atoi(parts[1])

	if service == "anilist" {
		data, err := getAniListDetails(id)
		if err != nil {
			return nil, err
		}
		return buildAniListEmbed(data), nil
	} else if strings.HasPrefix(service, "tmdb") {
		mediaType := strings.TrimPrefix(service, "tmdb_")
		if mediaType == "tv" {
			data, err := getTMDBTVDetails(id)
			if err != nil {
				return nil, err
			}
			return buildTMDBTVEmbed(data), nil
		} else {
			data, err := getTMDBMovieDetails(id)
			if err != nil {
				return nil, err
			}
			return buildTMDBMovieEmbed(data), nil
		}
	}
	return nil, fmt.Errorf("unknown service")
}

// calculateScore assigns a relevance score based on title matching
func calculateScore(query, target string) int {
	query = strings.ToLower(query)
	target = strings.ToLower(target)
	if query == target {
		return 1000
	}
	if strings.HasPrefix(target, query) {
		return 500
	}
	if strings.Contains(target, query) {
		return 100
	}
	return 0
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}

// interactionCreate handles the selection and buttons
func interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type == discordgo.InteractionApplicationCommand {
		if i.ApplicationCommandData().Name == "search" {
			query := i.ApplicationCommandData().Options[0].StringValue()
			options, topEmbed, err := searchAndPreview(query)
			if err != nil {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "No results found.",
					},
				})
				return
			}

			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Is this the correct media?",
					Embeds:  []*discordgo.MessageEmbed{topEmbed},
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{
							Components: []discordgo.MessageComponent{
								discordgo.SelectMenu{
									CustomID:    "media_select_manual",
									Placeholder: "Choose a different one...",
									Options:     options,
								},
							},
						},
						discordgo.ActionsRow{
							Components: []discordgo.MessageComponent{
								discordgo.Button{
									Label:    "Confirm",
									Style:    discordgo.SuccessButton,
									CustomID: "confirm_selection_manual",
								},
								discordgo.Button{
									Label:    "Cancel",
									Style:    discordgo.DangerButton,
									CustomID: "cancel_search",
								},
							},
						},
					},
				},
			})
		}
		return
	}

	if i.Type != discordgo.InteractionMessageComponent {
		return
	}

	customID := i.MessageComponentData().CustomID

	switch customID {
	case "media_select", "media_select_manual":
		selectedValue := i.MessageComponentData().Values[0]
		embed, err := getEmbedForValue(selectedValue)
		if err == nil {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
				Data: &discordgo.InteractionResponseData{
					Embeds:     []*discordgo.MessageEmbed{embed},
					Components: i.Message.Components,
				},
			})
		}

	case "confirm_selection":
		// Forum Watcher confirmation - just keep the embed
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "",
				Embeds:     i.Message.Embeds,
				Components: []discordgo.MessageComponent{},
			},
		})

	case "confirm_selection_manual":
		// Manual Search confirmation - Create a new forum post
		forumChannelID := os.Getenv("FORUM_CHANNEL_ID")
		if forumChannelID == "" {
			return
		}

		embed := i.Message.Embeds[0]
		imageURL := ""
		if embed.Thumbnail != nil {
			imageURL = embed.Thumbnail.URL
		}

		// Ack the interaction first (it's ephemeral, so we'll update it)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "Creating forum post...",
				Embeds:     []*discordgo.MessageEmbed{},
				Components: []discordgo.MessageComponent{},
			},
		})

		// 1. Download image for the attachment (to show in gallery view)
		var files []*discordgo.File
		if imageURL != "" {
			resp, err := http.Get(imageURL)
			if err == nil {
				defer resp.Body.Close()
				files = append(files, &discordgo.File{
					Name:        "poster.jpg",
					ContentType: "image/jpeg",
					Reader:      resp.Body,
				})
			}
		}

		// 2. Start forum thread with an initial message
		// Forum posts require a message to be sent as part of the thread creation.
		thread, err := s.ForumThreadStartComplex(forumChannelID, &discordgo.ThreadStart{
			Name:                truncate(embed.Title, 100),
			AutoArchiveDuration: 60,
		}, &discordgo.MessageSend{
			Content: "Media details for: **" + embed.Title + "**",
			Files:   files,
			Embeds:  []*discordgo.MessageEmbed{embed},
		})
		if err != nil {
			log.Printf("Error creating thread: %v\n", err)
			s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: pointer("Error creating forum post: " + err.Error()),
			})
			return
		}

		// 3. Update ephemeral response with a link to the new post
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: pointer("Added **" + embed.Title + "**! [Link](https://discord.com/channels/" + i.GuildID + "/" + thread.ID + ")"),
		})

	case "cancel_search":
		// Delete/update the preview message
		if i.Message.Flags == discordgo.MessageFlagsEphemeral {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
				Data: &discordgo.InteractionResponseData{
					Content:    "Search cancelled.",
					Embeds:     []*discordgo.MessageEmbed{},
					Components: []discordgo.MessageComponent{},
				},
			})
		} else {
			s.ChannelMessageDelete(i.ChannelID, i.Message.ID)
		}
	}
}

func buildAniListEmbed(media *AniListMedia) *discordgo.MessageEmbed {
	desc := media.Description
	if len(desc) > 350 {
		desc = desc[:350] + "..."
	}
	desc = strings.ReplaceAll(desc, "<br>", "\n")
	desc = strings.ReplaceAll(desc, "<i>", "*")
	desc = strings.ReplaceAll(desc, "</i>", "*")

	fields := []*discordgo.MessageEmbedField{
		{Name: "Rating", Value: fmt.Sprintf("%d/100", media.AverageScore), Inline: true},
		{Name: "Year", Value: fmt.Sprintf("%d", media.SeasonYear), Inline: true},
		{Name: "Genres", Value: strings.Join(media.Genres, ", "), Inline: true},
		{Name: "Episodes", Value: fmt.Sprintf("%d", media.Episodes), Inline: true},
		{Name: "Duration", Value: fmt.Sprintf("%d mins", media.Duration), Inline: true},
		{Name: "Status", Value: media.Status, Inline: true},
	}

	// Add relations (Seasons/Prequels/Sequels)
	var relations []string
	for _, edge := range media.Relations.Edges {
		if edge.Node.Type == "ANIME" && (edge.RelationType == "PREQUEL" || edge.RelationType == "SEQUEL") {
			name := edge.Node.Title.English
			if name == "" {
				name = edge.Node.Title.Romaji
			}
			relations = append(relations, fmt.Sprintf("%s: %s", strings.Title(strings.ToLower(edge.RelationType)), name))
		}
	}
	if len(relations) > 0 {
		if len(relations) > 3 {
			relations = relations[:3]
			relations = append(relations, "...")
		}
		fields = append(fields, &discordgo.MessageEmbedField{
			Name: "Related / Other Seasons", Value: strings.Join(relations, "\n"), Inline: false,
		})
	}

	return &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("%s (%s)", media.Title.English, media.Title.Romaji),
		Description: desc,
		URL:         media.SiteUrl,
		Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: media.CoverImage.Large},
		Fields:      fields,
		Color:       0x3DB4F2,
	}
}

func buildTMDBTVEmbed(data *TMDBTVDetails) *discordgo.MessageEmbed {
	year := data.FirstAirDate
	if len(year) > 4 {
		year = year[:4]
	}

	duration := "N/A"
	if len(data.EpisodeRunTime) > 0 {
		duration = fmt.Sprintf("%d mins", data.EpisodeRunTime[0])
	}

	// Add genres
	genres := []string{}
	for _, g := range data.Genres {
		genres = append(genres, g.Name)
	}

	return &discordgo.MessageEmbed{
		Title:       data.Name,
		Description: data.Overview,
		Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: "https://image.tmdb.org/t/p/w500" + data.PosterPath},
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Rating", Value: fmt.Sprintf("%.1f/10", data.VoteAverage), Inline: true},
			{Name: "Year", Value: year, Inline: true},
			{Name: "Genres", Value: strings.Join(genres, ", "), Inline: true},
			{Name: "Seasons", Value: fmt.Sprintf("%d", data.NumberOfSeasons), Inline: true},
			{Name: "Episodes", Value: fmt.Sprintf("%d", data.NumberOfEpisodes), Inline: true},
			{Name: "Avg. Duration", Value: duration, Inline: true},
			{Name: "Type", Value: "TV Show", Inline: true},
		},
		Color: 0x01D277,
	}
}

func buildTMDBMovieEmbed(data *TMDBMovieDetails) *discordgo.MessageEmbed {
	year := data.ReleaseDate
	if len(year) > 4 {
		year = year[:4]
	}
	// Add genres
	genres := []string{}
	for _, g := range data.Genres {
		genres = append(genres, g.Name)
	}

	return &discordgo.MessageEmbed{
		Title:       data.Title,
		Description: data.Overview,
		Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: "https://image.tmdb.org/t/p/w500" + data.PosterPath},
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Rating", Value: fmt.Sprintf("%.1f/10", data.VoteAverage), Inline: true},
			{Name: "Year", Value: year, Inline: true},
			{Name: "Genres", Value: strings.Join(genres, ", "), Inline: true},
			{Name: "Runtime", Value: fmt.Sprintf("%d mins", data.Runtime), Inline: true},
			{Name: "Type", Value: "Movie", Inline: true},
		},
		Color: 0x01D277,
	}
}

func pointer[T any](v T) *T { return &v }