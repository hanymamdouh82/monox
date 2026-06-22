# Output configuration
Output demo.gif
Output demo.mp4

# Terminal layout styling
Set Shell "zsh"
Set FontSize 14
Set Width 1920
Set Height 1080
Set Padding 20
Set Theme "Catppuccin Mocha"

# Simulation script
Type "monox -config /mnt/repos/misc/monox/config.yaml"
Enter
Sleep 1.5s

# Allow the TUI panels to cycle through a couple of ticks live
Sleep 20s

# Gracefully exit the application
Type "q"
Sleep 500ms
