#!/usr/bin/env bash
# Hiding and sizing the sidebar, end to end. Its width is the normal server's
# @wtui-width option; dragging the border sets another for the launch, and
# resizing the terminal keeps it. z hides the sidebar and the wtui key brings
# it back. Under 100 columns it hides, and it shows again once the terminal
# widens, unless z hid it.
set -euo pipefail
# shellcheck source=test/lib/host.sh
. "$(dirname "$0")/lib/host.sh"
need_host_tmux
build_wtui

new_session alpha

# the width is the normal server's @wtui-width option, read at launch
tmux set-option -g @wtui-width 40
launch t1
[ "$(sidebar_width t1)" = 40 ] || fail "with @wtui-width 40, the sidebar is $(sidebar_width t1) columns wide"

# Resizing the terminal keeps the sidebar's width, and dragging the border
# (column 41) sets a new one for the launch, which resizing keeps too.
width_is() { [ "$(sidebar_width t1)" = "$1" ]; }
resize_terminal t1 150
wait_until "the sidebar to keep its 40 columns at 150" width_is 40
drag t1 41 10 35 10
wait_until "dragging the border to make the sidebar 34 columns wide" width_is 34
resize_terminal t1 110
wait_until "the sidebar to keep the dragged 34 columns at 110" width_is 34
resize_terminal t1 140
wait_until "the sidebar to keep the dragged 34 columns at 140" width_is 34

# z hides the sidebar: the right side has the whole terminal, and focus.
# The wtui key brings the sidebar back, as wide as it was, with focus.
right_width() { host t1 display-message -p -t "$(right_pane t1)" '#{pane_width}'; }
[ "$(focus t1)" = sidebar ] || fail "focus is on the $(focus t1) before z, want the sidebar"
type_in t1 z
wait_until "z to hide the sidebar" hidden t1
wait_until "z to focus the right side" focus_on t1 right
[ "$(right_width)" = 140 ] || fail "with the sidebar hidden, the right side is $(right_width) columns wide, want 140"
case "$(screen t1)" in *' wtui '*) fail "with the sidebar hidden, the terminal shows it:
$(screen t1)" ;; esac
type_in t1 'C-\'
wait_until "the wtui key to show the sidebar" eval '! hidden t1'
wait_until "the wtui key to focus the sidebar" focus_on t1 sidebar
[ "$(sidebar_width t1)" = 34 ] || fail "the sidebar came back $(sidebar_width t1) columns wide, want 34"

# Under 100 columns the sidebar hides, and at 100 or more it shows again, as
# wide as it was, with focus where it was: on the right.
type_in t1 Escape
wait_until "esc to focus the right side" focus_on t1 right
resize_terminal t1 99
wait_until "a 99-column terminal to hide the sidebar" hidden t1
resize_terminal t1 80
hidden t1 || fail "the sidebar shows in an 80-column terminal"
resize_terminal t1 100
wait_until "a 100-column terminal to show the sidebar" eval '! hidden t1'
wait_until "the sidebar to come back 34 columns wide" width_is 34
[ "$(focus t1)" = right ] || fail "showing the sidebar again moved focus to the $(focus t1)"
resize_terminal t1 130
wait_until "the sidebar to keep its 34 columns at 130" width_is 34

# Hidden with z, the sidebar stays hidden as the terminal narrows and widens
# again, until the wtui key.
type_in t1 'C-\'
wait_until "the wtui key to focus the sidebar" focus_on t1 sidebar
type_in t1 z
wait_until "z to hide the sidebar" hidden t1
resize_terminal t1 90
resize_terminal t1 120
sleep 0.5 # the hooks have had their chance to show it
hidden t1 || fail "the sidebar hidden with z came back as the terminal widened"
type_in t1 'C-\'
wait_until "the wtui key to show the sidebar" eval '! hidden t1'
wait_until "the sidebar to come back 34 columns wide" width_is 34
focus_on t1 sidebar || fail "the wtui key showed the sidebar with focus on the $(focus t1)"

# A terminal too narrow for the sidebar's width doesn't change it: shown
# there with the wtui key, the sidebar is as wide as fits (leaving the right
# side a column), and once the terminal is wide again it's 34 columns again.
type_in t1 Escape
wait_until "esc to focus the right side" focus_on t1 right
resize_terminal t1 30
wait_until "a 30-column terminal to hide the sidebar" hidden t1
type_in t1 'C-\'
wait_until "the wtui key to show the sidebar in a 30-column terminal" eval '! hidden t1'
wait_until "the sidebar to be as wide as fits in 30 columns, 28" width_is 28
resize_terminal t1 120
wait_until "the sidebar to be 34 columns wide again at 120" width_is 34

# Launched in a narrow terminal, the sidebar starts hidden, with focus on the
# right. Widened, it shows, and shows that it hasn't focus (no cursor) until
# the wtui key gives it focus.
term_cols=90 launch t2
hidden t2 || fail "launched in a 90-column terminal, the sidebar shows"
[ "$(focus t2)" = right ] || fail "launched hidden, focus is on the $(focus t2)"
resize_terminal t2 120
wait_until "a 120-column terminal to show t2's sidebar" eval '! hidden t2'
wait_until "t2's sidebar to be 40 columns wide" eval '[ "$(sidebar_width t2)" = 40 ]'
wait_until "t2's sidebar to draw its tree" eval 'sidebar_text t2 | grep alpha >/dev/null'
[ -z "$(cursor_row t2)" ] || fail "launched hidden then shown, the sidebar shows its cursor on '$(cursor_row t2)' without focus"
type_in t2 'C-\'
wait_until "the wtui key to focus t2's sidebar" focus_on t2 sidebar
wait_until "t2's sidebar to show its cursor on alpha" eval '[ "$(cursor_row t2)" = "1   ▾ alpha" ]'

echo "ok: wtui-host-hide"
