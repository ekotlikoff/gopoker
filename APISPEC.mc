- GET /
    - Get static webassembly client
- GET /group
    - Get the list of groups you are a member of
- POST /group
    - Create a new group (group name)
- POST /group/{id}
    Modify a group (add/remove member, modify member permissions)
- GET /group/{id}
    - Get the information for a group (tables, members, etc)
- POST /group/{id}/table
    - Create a new table
- POST /group/{id}/table/{id}
    - Modify a table (table config change - e.g. TimeToBidMultiplier, MinTimeToBid, Blinds, AllowBlindModification, etc)
- GET /group/{id}/table/{id}
    - Initiate websocket connection for the group's table
    - Websocket message spec:
        - Client message types:
            - Sitdown (seat number)
            - Standup
            - Turn (bet amount, fold)
            - BlindModification
            - StartGame/PauseGame
        - Server message types:
            - Hand (2 card hand)
            - Turn (bet amount, fold, playerID)
            - ConfigChange (table config, see POST /table/{id})
    Websocket pseudo code:
        - Server listener:
            - Sitdown: end sit request to the table's sitdown channel
            - Standup: send standup request to table's standup channel
            - Turn: send turn via player's turnChan
            - BlindModification: send modification to table's config channel
            - StartGame/PauseGame, call table's start/pause handler
        - Table:
            - Table Sitdown listener
                - player := <-sitdownChan, if player sitting down, player.toFold = false
                - else if seat not filled, add(player)
            - Table Standup listener
                - <-standupChan, if player.inHand, player.toFold = true
            - Table loop:
                - Hand loop:
                    - for player # Play hand
                        - # Handle player's turn
                            - if player.toFold, remove(player)
                            - handlePlayerTurn(player)
                    - Deal next card
                - for player, player.inHand = false
                - handleConfigChange(<-configChan)
                - waitForSufficientPlayers()
            - handlePlayerTurn: wait on turnChan or pauseChan or timeout - player.timeTaken
                - if timeout, set player.timeTaken
                - if turnChan, handle turn, if success set timeTaken = 0
                - if timeout, remove(player)

	google.golang.org/protobuf v1.28.0 // indirect
