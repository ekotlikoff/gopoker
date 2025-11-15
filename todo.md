### TODO
1. Make chip count prettier
1. Make pot prettier (maybe 5 thresholds, above which there are 5 different visualizations of chip stacks)
1. Make it clear which player you are
1. Visualize winner after a hand with chip sliding animation
1. Add check/call/raise/fold animations
1. Add table pause functionality
1. Add ability to configure table when creating
1. Show cards on all-in
1. Show winner's cards

### Done
1. Hide cards after folding
1. Make calls and checks and folds clear to other players
1. Add key status messages in the text box (who winner was, someone stands up, etc)

Previously the model was doing too much, we had control flow/game orchestration going on as well.

Instead, we want the model to just represent a game and you can do things to it, e.g. a player can try and take an action (and either get an error or not back).

On top of the model we will have the `tableserver` which orchestrates the game and handles timeouts etc. There will be no need for a mutex in the model because only one thread will ever touch it (the `tableserver` thread), the various client threads will communicate with the tableserver via channels.
