Previously the model was doing too much, we had control flow/game orchestration going on as well.

Instead, we want the model to just represent a game and you can do things to it, e.g. a player can try and take an action (and either get an error or not back).

On top of the model we will have the `tableserver` which orchestrates the game and handles timeouts etc. There will be no need for a mutex in the model because only one thread will ever touch it (the `tableserver` thread), the various client threads will communicate with the tableserver via channels.
