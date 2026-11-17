# Keystrokes for .github/assets/demo-{dark,light}.gif. Run through scripts/demo.sh, which
# sets up the made-up hosts this expects and prepends Output and Set Theme per recording.
Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 560
Set Padding 16
Set TypingSpeed 0

# Start the UI with staging's forward running.
Hide
Type "export HOME=$LAZYFWD_DEMO_HOME; (sleep 1; lazyfwd forward start 1) >/dev/null 2>&1 & clear; lazyfwd"
Enter
Sleep 2s
Show
Sleep 1.5s

# Browse the hosts; Forwards follows the selection.
Down@700ms 3
Sleep 700ms
Up@500ms 3
Sleep 800ms

# Open gpu-box, then its listening ports.
Enter
Sleep 1s
Tab
Sleep 1.5s

# Forward jupyter-lab's port 8888.
Down@500ms 3
Sleep 800ms
Space
Sleep 2s

# Stop the new forward, then start it again.
Type "2"
Sleep 1.2s
Space
Sleep 1.8s
Space
Sleep 2.5s

Hide
Type "q"
Type "y"
Sleep 1s
