# Configuration & Usage

MailBot provides an interactive CLI menu to guide you through setup.

## Menu Options

**Main Menu:**
1.  **Start Service**: Launches the IMAP listener. Logs will be displayed live in the console.
2.  **Config Wizard**: Add or remove email accounts.
3.  **Bot Settings**: Manage your Telegram Bot configuration.
4.  **AI Settings**: Configure AI analysis model, API provider, and behavior rules.
5.  **System Settings**: Advanced configuration (polling, retries, logs).
6.  **Test Connection**: Verify your Telegram setup.
7.  **Exit**: Quit the application.

---

## Detailed Settings

### 1. Config Wizard (Accounts)

Add or manage your IMAP accounts here. You will need:
*   **Provider**: Gmail (with App Password), Outlook, etc.
*   **Email Address**: Your full email address.
*   **Password**: Your **App Password** (not your login password).
*   **IMAP Settings**: Usually auto-detected for common providers.

### 2. Bot Settings

Configure your Telegram notification channel.
*   **Start the Bot**: Search for your bot in Telegram and start a chat.
*   **Enter User ID**: Your Telegram user ID (Chat ID).
*   **Enter Token**: The API token from BotFather.

### 3. AI Settings

Configure AI analysis through LiteLLM with a two-level menu.
*   **Enable/Disable**: Turn AI analysis on or off.
*   **Provider Group → Platform**: Pick a group, then a specific platform. Presets cover OpenAI/OpenRouter/Together/Fireworks, Anthropic/Gemini/Mistral/Groq, DeepSeek/Qwen/Moonshot/Minimax, Perplexity/Cohere, plus Ollama and custom OpenAI-compatible endpoints.
*   **API Key**: Prompted only when required by the selected platform.
*   **Model**: Override the default model name shown in the menu.
*   **Base URL**: Editable for Ollama, OpenRouter, Together, Fireworks, and custom OpenAI-compatible endpoints.
*   **Default Mode & Language**: Choose the email processing mode:
    - **Raw**: Simple forwarding without AI analysis (zero token cost)
    - **Hybrid**: Email preview with optional AI analysis (**Recommended for cost-conscious users**)
      - Email preview + three buttons: Summary, Original, Translate
      - Summary button: On-demand AI analysis (costs tokens only when clicked)
      - Original button: Show full email content (no cost, uses cached email)
      - Translate button: Appears only if email language differs from target (costs tokens when clicked)
      - **Savings**: 80-90% token savings vs Agent mode for typical users
    - **Agent**: Automatic AI summary on every email (costs tokens upfront)
      - Automatic AI summary for every email
      - Original button: Show full email (no cost)
      - Translate button: Manual translation (costs tokens when clicked)
    
    Also select the output language (auto-detect, or override to a specific language).

### 4. Rules System

You can manage AI behavior rules dynamically via Telegram, without editing files manually.
*   Send `/rules` in Telegram to view active rules.
*   Follow the bot's prompts to add or remove rules using natural language.
*   These rules are persisted to `rules.md` and injected into the AI prompt.
*   **Rule Execution**: Rules are evaluated in the order they appear in `rules.md`. If multiple rules match the same email, all applicable rules are executed sequentially.
*   **Rule Priority**: More specific rules (e.g., matching specific sender + keyword) should be placed before broader rules to avoid conflicts.

Examples:
> "Add rule: Ignore all emails from no-reply@example.com"
> "Add rule: If the email contains a verification code, extract and bold it"
> "Add rule: Flag emails about project deadlines with ⚠️"

### 5. System Settings

Customize the behavior of the mail fetcher.
*   **Polling Interval**: How often (in seconds) to check for new emails. (Default: 60s, Min: 10s)
    - **Recommendations**:
      - **10-30s**: For critical accounts where immediate notification is important (e.g., security alerts, urgent work emails)
      - **60s**: Balanced setting for most users. Good for normal inbox monitoring.
      - **300s+ (5+ min)**: For low-volume accounts or when you prefer less frequent checks to reduce API usage.
*   **Max Retries**: Number of retry attempts on network failure before giving up on a fetch cycle. (Default: 3, Recommended: 2-5)
    - Higher values increase resilience on unreliable networks but may delay error detection.
*   **Log Level**: Verbosity of the console output (DEBUG, INFO, WARNING, ERROR). 
    - Set to **DEBUG** for troubleshooting connection issues or API errors.
    - Use **INFO** for normal operation (recommended for production).

---

## Running Headless

For servers or Docker environments without interactivity.

1.  Set up your configuration using the interactive wizard first.
2.  Run with the `--headless` flag:
    ```bash
    python main.py --headless
    ```
    or specify a custom config file:
    ```bash
    python main.py --headless -c /path/to/my_config.json
    ```
