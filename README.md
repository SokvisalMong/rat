# rat
A Discord bot that automatically searches for media details (Anime, Movies, Dramas) when new forum posts are created.

## Features
- **Forum Watcher**: Monitors a specific forum channel for new posts.
- **Anime Search**: Powered by AniList (No API key required).
- **Movie/TV Search**: Powered by TMDB (API key required).
- **Rich Embeds**: Automatically posts detailed info (ratings, year, episodes, etc.) into the new thread.

## Setup

### Local Setup
1. **Install Dependencies**:
   ```bash
   go mod tidy
   ```
2. **Configuration**:
   Create a `.env` file in the root directory:
   ```env
   DISCORD_TOKEN=your_bot_token
   TMDB_API_KEY=your_tmdb_api_key
   FORUM_CHANNEL_ID=your_forum_channel_id
   GUILD_ID=your_guild_id
   ```
3. **Run**:
   ```bash
   go run .
   ```

### Docker Setup
1. **Configure Environment**: Ensure `.env` is populated.
2. **Build and Run**:
   ```bash
   docker-compose up --build -d
   ```

### Slash Commands
- `/search <query>`: Manually search for media and create a new forum post with the details and poster.

## Inviting the Bot
To invite the bot, go to the **Discord Developer Portal** > **OAuth2** > **URL Generator** and select the following:

### Scopes
- `bot`
- `applications.commands` (Required for the `/search` command)

### Bot Permissions
Ensure these permissions are checked:
- **General Permissions**:
  - `View Channels`
- **Text Permissions**:
  - `Send Messages`
  - `Create Public Threads`
  - `Send Messages in Threads`
  - `Embed Links` (Required for media details)
  - `Attach Files` (Required for uploading posters)
  - `Read Message History`

### Gateway Intents
Ensure documentation in the **Bot** tab of the Developer Portal has the following **Privileged Gateway Intents** enabled:
- **Message Content Intent**: Required to read the forum post title for searching.