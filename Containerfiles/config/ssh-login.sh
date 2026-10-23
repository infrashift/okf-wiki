#!/bin/bash
# ForceCommand handler. A command given to ssh runs as asked — `ssh host cmd`,
# legacy `scp -O`, and the sftp subsystem modern scp and sftp use (sshd hands
# its path over as the command). Only an interactive login attaches to the
# tmux dev session; attaching every connection broke scp and remote commands.

# sshd does not pass on the image's ENV, so put okfw and friends on PATH here.
# Not a login shell: scp and sftp break on anything a profile prints.
export PATH="$HOME/.local/bin:$PATH"

if [[ -n "${SSH_ORIGINAL_COMMAND:-}" ]]; then
    exec /bin/bash -c "$SSH_ORIGINAL_COMMAND"
fi

export TERM=xterm-256color
SESSION="dev"

if tmux has-session -t "$SESSION" 2>/dev/null; then
    exec tmux attach -t "$SESSION"
fi

# Create the standard dev layout (same as start-session.sh but with fixed dimensions)
tmux new-session -d -s "$SESSION" -x 200 -y 50 nvim
tmux split-window -h -t "$SESSION" -p 35 /bin/bash
tmux split-window -v -t "$SESSION" claude
tmux select-pane -t "$SESSION:0.0"
exec tmux attach -t "$SESSION"
