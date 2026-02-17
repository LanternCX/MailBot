# Configuration & Usage

MailBot provides an interactive CLI menu to guide you through setup.

## Menu Options

**Main Menu:**
1.  **Start Service**: Launches the IMAP listener. Logs will be displayed live in the console.
2.  **Config Wizard**: Add or remove email accounts.
3.  **Bot Settings**: Manage your Telegram Bot configuration.
4.  **System Settings**: Advanced configuration (polling, retries, logs).
5.  **Test Connection**: Verify your Telegram setup.
6.  **Exit**: Quit the application.

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

### 3. System Settings

Customize the behavior of the mail fetcher.
*   **Polling Interval**: How often (in seconds) to check for new emails. (Default: 60s, Min: 10s)
*   **Max Retries**: Number of retry attempts on network failure before giving up on a fetch cycle. (Default: 3)
*   **Log Level**: Verbosity of the console output (DEBUG, INFO, WARNING, ERROR). Set to DEBUG for troubleshooting connection issues.

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
