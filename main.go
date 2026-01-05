package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"gator/internal/config"
	"gator/internal/database"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

type state struct {
	cfg *config.Config
	db  *database.Queries
}

type command struct {
	name string
	args []string
}

type commands struct {
	handlers map[string]func(*state, command) error
}

func (c *commands) register(name string, f func(*state, command) error) {
	c.handlers[name] = f
}

func (c *commands) run(s *state, cmd command) error {
	handler, ok := c.handlers[cmd.name]
	if !ok {
		return fmt.Errorf("unknown command: %s", cmd.name)
	}
	return handler(s, cmd)
}

// chapter 4 part 2, middleware
func middlewareLoggedIn(
	handler func(s *state, cmd command, user database.User) error,
) func(*state, command) error {
	return func(s *state, cmd command) error {
		currentUser := s.cfg.CurrentUserName
		if currentUser == "" {
			return errors.New("no current user set (run login first)")
		}

		user, err := s.db.GetUser(context.Background(), currentUser)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("user %s does not exist", currentUser)
			}
			return err
		}

		return handler(s, cmd, user)
	}
}

func handlerRegister(s *state, cmd command) error {
	if len(cmd.args) < 1 {
		return errors.New("register requires a username")
	}

	name := cmd.args[0]
	now := time.Now()

	user, err := s.db.CreateUser(context.Background(), database.CreateUserParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		Name:      name,
	})
	if err != nil {
		// Unique violation: name already exists -> exit(1)
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return fmt.Errorf("user %s already exists", name)
		}
		return err
	}

	if err := s.cfg.SetUser(name); err != nil {
		return err
	}

	fmt.Printf("user %s created\n", name)
	log.Printf("created user: %+v\n", user)
	return nil
}

func handlerLogin(s *state, cmd command) error {
	if len(cmd.args) < 1 {
		return errors.New("login requires a username")
	}

	username := cmd.args[0]

	// Must error if user doesn't exist
	_, err := s.db.GetUser(context.Background(), username)
	if err != nil {
		// sqlc usually returns sql.ErrNoRows when not found
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("user %s does not exist", username)
		}
		return err
	}

	if err := s.cfg.SetUser(username); err != nil {
		return err
	}

	fmt.Printf("current user set to %s\n", username)
	return nil
}

func handlerReset(s *state, cmd command) error {
	// No args required
	if err := s.db.ResetUsers(context.Background()); err != nil {
		return err
	}

	fmt.Println("database reset successful")
	return nil
}

func handlerUsers(s *state, cmd command) error {
	users, err := s.db.GetUsers(context.Background())
	if err != nil {
		return err
	}

	for _, u := range users {
		if u.Name == s.cfg.CurrentUserName {
			fmt.Printf("%s (current)\n", u.Name)
		} else {
			fmt.Printf("%s\n", u.Name)
		}
	}

	return nil
}

// for chapter 3 part 1, website was recommended to be used: https://www.wagslane.dev/index.xml
func handlerAgg(s *state, cmd command) error {
	if len(cmd.args) != 1 {
		return errors.New("agg requires a time_between_reqs (e.g. 1s, 1m, 1h)")
	}

	timeBetweenRequests, err := time.ParseDuration(cmd.args[0])
	if err != nil {
		return err
	}

	fmt.Printf("Collecting feeds every %s\n", timeBetweenRequests)

	ticker := time.NewTicker(timeBetweenRequests)
	defer ticker.Stop()

	for ; ; <-ticker.C {
		if err := scrapeFeeds(s); err != nil {
			// don’t crash the loop on one bad feed
			fmt.Fprintln(os.Stderr, "error scraping feeds:", err)
		}
	}
}

func handlerAddFeed(s *state, cmd command, user database.User) error {
	if len(cmd.args) < 2 {
		return errors.New("addfeed requires a name and url")
	}

	feedName := cmd.args[0]
	feedURL := cmd.args[1]

	now := time.Now()
	feed, err := s.db.CreateFeed(context.Background(), database.CreateFeedParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		Name:      feedName,
		Url:       feedURL,
		UserID:    user.ID,
	})
	if err != nil {
		// Unique violation (url already exists)
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return fmt.Errorf("feed url already exists: %s", feedURL)
		}
		return err
	}

	// Automatically follow the feed for the current user
	now2 := time.Now()
	_, err = s.db.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
		ID:        uuid.New(),
		CreatedAt: now2,
		UpdatedAt: now2,
		UserID:    user.ID,
		FeedID:    feed.ID,
	})
	if err != nil {
		// Ignore duplicate follows
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			// already following — fine
		} else {
			return err
		}
	}

	// Print out the fields of the new feed record
	fmt.Println("feed created:")
	fmt.Printf("  id: %s\n", feed.ID)
	fmt.Printf("  created_at: %v\n", feed.CreatedAt)
	fmt.Printf("  updated_at: %v\n", feed.UpdatedAt)
	fmt.Printf("  name: %s\n", feed.Name)
	fmt.Printf("  url: %s\n", feed.Url)
	fmt.Printf("  user_id: %s\n", feed.UserID)

	return nil
}

func handlerFollow(s *state, cmd command) error {
	if len(cmd.args) != 1 {
		return errors.New("follow requires a url")
	}
	feedURL := cmd.args[0]

	currentUser := s.cfg.CurrentUserName
	if currentUser == "" {
		return errors.New("no current user set")
	}

	user, err := s.db.GetUser(context.Background(), currentUser)
	if err != nil {
		return err
	}

	feed, err := s.db.GetFeedByURL(context.Background(), feedURL)
	if err != nil {
		return err
	}

	now := time.Now()
	ff, err := s.db.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		UserID:    user.ID,
		FeedID:    feed.ID,
	})
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return fmt.Errorf("%s already follows %s", user.Name, feed.Name)
		}
		return err
	}

	fmt.Printf("%s now follows %s\n", ff.UserName, ff.FeedName)
	return nil
}

func handlerFollowing(s *state, cmd command) error {
	if len(cmd.args) != 0 {
		return errors.New("following takes no arguments")
	}

	currentUser := s.cfg.CurrentUserName
	if currentUser == "" {
		return errors.New("no current user set")
	}

	user, err := s.db.GetUser(context.Background(), currentUser)
	if err != nil {
		return err
	}

	follows, err := s.db.GetFeedFollowsForUser(context.Background(), user.ID)
	if err != nil {
		return err
	}

	for _, f := range follows {
		fmt.Printf("* %s\n", f.FeedName)
	}
	return nil
}

func handlerUnfollow(s *state, cmd command, user database.User) error {
	if len(cmd.args) != 1 {
		return errors.New("unfollow requires a url")
	}
	feedURL := cmd.args[0]

	feed, err := s.db.GetFeedByURL(context.Background(), feedURL)
	if err != nil {
		return err
	}

	// delete follow by (user_id, feed_id)
	err = s.db.DeleteFeedFollow(context.Background(), database.DeleteFeedFollowParams{
		UserID: user.ID,
		FeedID: feed.ID,
	})
	if err != nil {
		return err
	}

	fmt.Printf("%s unfollowed %s\n", user.Name, feed.Name)
	return nil
}

func handlerFeeds(s *state, cmd command) error {
	if len(cmd.args) != 0 {
		return errors.New("feeds takes no arguments")
	}

	feeds, err := s.db.GetFeeds(context.Background())
	if err != nil {
		return err
	}

	for _, f := range feeds {
		fmt.Printf("* %s\n", f.Name)
		fmt.Printf("  %s\n", f.Url)
		fmt.Printf("  added by: %s\n", f.UserName)
	}

	return nil
}

func scrapeFeeds(s *state) error {
	feed, err := s.db.GetNextFeedToFetch(context.Background())
	if err != nil {
		return err
	}

	// mark fetched first (per assignment)
	if err := s.db.MarkFeedFetched(context.Background(), feed.ID); err != nil {
		return err
	}

	fmt.Printf("fetching feed: %s (%s)\n", feed.Name, feed.Url)

	rss, err := fetchFeed(context.Background(), feed.Url)
	if err != nil {
		return err
	}
	// posts section updated, chapter 5 part 2
	for _, item := range rss.Channel.Item {
		now := time.Now()

		// description nullable
		desc := sql.NullString{Valid: false}
		if item.Description != "" {
			desc = sql.NullString{String: item.Description, Valid: true}
		}

		// published_at nullable
		var publishedAt sql.NullTime
		if t, ok := parsePubDate(item.PubDate); ok {
			publishedAt = sql.NullTime{Time: t, Valid: true}
		}

		_, err := s.db.CreatePost(context.Background(), database.CreatePostParams{
			ID:          uuid.New(),
			CreatedAt:   now,
			UpdatedAt:   now,
			Title:       item.Title,
			Url:         item.Link,
			Description: desc,
			PublishedAt: publishedAt,
			FeedID:      feed.ID,
		})
		if err != nil {
			// Ignore duplicate URL errors
			if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
				continue
			}
			log.Printf("error creating post (url=%s): %v", item.Link, err)
		}
	}

	return nil
}

func handlerBrowse(s *state, cmd command, user database.User) error {
	limit := int32(2)
	if len(cmd.args) >= 1 {
		n, err := strconv.Atoi(cmd.args[0])
		if err != nil || n <= 0 {
			return fmt.Errorf("browse limit must be a positive integer")
		}
		limit = int32(n)
	}

	posts, err := s.db.GetPostsForUser(context.Background(), database.GetPostsForUserParams{
		UserID: user.ID,
		Limit:  limit,
	})
	if err != nil {
		return err
	}

	for _, p := range posts {
		fmt.Println("-------------------------------------------------")
		fmt.Println(p.Title)
		fmt.Println(p.Url)

		if p.PublishedAt.Valid {
			fmt.Println("Published:", p.PublishedAt.Time)
		}
		if p.Description.Valid {
			fmt.Println()
			fmt.Println(p.Description.String)
		}
	}
	fmt.Println("-------------------------------------------------")
	return nil
}

func main() {
	// Require: program name + command name at minimum
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "not enough arguments provided")
		os.Exit(1)
	}

	// Read config once at startup
	cfg, err := config.Read()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Open DB connection
	dbURL := cfg.DBURL // if your field name differs, change this line to match
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()

	dbQueries := database.New(db)

	s := &state{
		cfg: &cfg,
		db:  dbQueries,
	}

	cmds := &commands{
		handlers: make(map[string]func(*state, command) error),
	}
	cmds.register("login", handlerLogin)
	cmds.register("register", handlerRegister)
	cmds.register("reset", handlerReset)
	cmds.register("users", handlerUsers)
	cmds.register("agg", handlerAgg)
	cmds.register("addfeed", middlewareLoggedIn(handlerAddFeed))
	cmds.register("feeds", handlerFeeds)
	cmds.register("follow", handlerFollow)
	cmds.register("following", handlerFollowing)
	cmds.register("unfollow", middlewareLoggedIn(handlerUnfollow))
	cmds.register("browse", middlewareLoggedIn(handlerBrowse))

	cmdName := os.Args[1]
	cmdArgs := os.Args[2:]

	cmd := command{
		name: cmdName,
		args: cmdArgs,
	}

	if err := cmds.run(s, cmd); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
