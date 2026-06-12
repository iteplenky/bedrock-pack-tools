# demo.tape - records docs/demo.gif for the README.
#
# Run from the repo root:  vhs .tape/demo.tape
#
# The flow runs against an ISOLATED $HOME (/Users/Shared/demo) holding a
# placeholder token and an empty store, so the recording never touches the
# network and never shows a real account, key, or server. It walks the
# interactive menu: the language toggle (English <-> Russian, in place) and
# entering a server address to reach the action picker. It never opens
# Featured (the only network path) and never starts a run.

Output docs/demo.gif

Set Shell "bash"
Set FontSize 16
Set Width 1000
Set Height 640
Set Padding 20
Set TypingSpeed 55ms

# --- isolated, offline, no-secrets setup (not recorded) ---
Hide
Type `export HOME=/Users/Shared/demo; export PS1='$ '`
Enter
Type `cd /Users/Shared/demo && clear`
Enter
Show

# --- launch the interactive menu ---
Type `./bedrock-pack-tools`
Sleep 400ms
Enter
Sleep 1700ms

# walk down to Settings (last section)
Down 6
Sleep 800ms
Enter
Sleep 700ms

# move up to the Language row and flip English <-> Russian in place
Up 6
Sleep 400ms
Enter
Sleep 1700ms
Enter
Sleep 1100ms

# back to the menu, then into "Enter a server address"
Escape
Sleep 500ms
Up 5
Sleep 600ms
Enter
Sleep 500ms
Type `play.example.net:19132`
Sleep 600ms
Enter
Sleep 1900ms

# done
Ctrl+C
Sleep 600ms
