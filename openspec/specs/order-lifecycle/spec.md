# order-lifecycle Specification

## Purpose
Tracks an order from the moment the user takes it on until it is confirmed complete. Successive world saves are used to detect when the work has actually been done, so the user never has to move cars in the app.

## Requirements

### Requirement: Order states
Every order SHALL be in exactly one of these states: Available, In Progress, Completion Detected, Completed or Cancelled. Only Available orders are regenerated. Every other state SHALL be persisted and survive restarts.

#### Scenario: Restart mid-session
- **WHEN** the app restarts while an order is In Progress
- **THEN** the order is still In Progress with the same cars

### Requirement: Starting an order
The user SHALL be able to start an Available order, which moves it to In Progress and freezes its car list as it stands in the current snapshot. For a local job order, the user SHALL also be able to start a single industry line. Starting a line creates an In Progress order containing only that line and leaves the job's other lines available.

#### Scenario: Start whole job
- **WHEN** the user starts the AB45 job order
- **THEN** it becomes In Progress, and its cars are reserved from all other orders

#### Scenario: Start one industry
- **WHEN** the user starts only the HTI line of the AB45 order
- **THEN** an In Progress order for AB45/HTI is created, and the remaining AB45 lines stay available

### Requirement: Completion detection for local jobs
A local job order SHALL move to Completion Detected when, in a newer snapshot, all of the following hold:
- every industry set-out is located at its industry;
- every yard-line set-out has reached its satellite yard, meaning it is in that yard or at an industry served by a job homed there;
- every pick-up is located in the job's home yard.

Progress SHALL be shown for each car and each line before then.

#### Scenario: Job worked and returned
- **WHEN** a new save shows all HTI set-outs at HTI and all HTI pick-ups on Norris yard tracks
- **THEN** the AB45/HTI order moves to Completion Detected

#### Scenario: Partial progress
- **WHEN** the set-outs are at HTI but the pick-ups are still on line
- **THEN** the order stays In Progress, with set-outs shown done and pick-ups pending

#### Scenario: Yard line completed
- **WHEN** A55's Tuscaloosa set-outs are on Tuscaloosa yard tracks or already spotted at A37 industries, and its Tuscaloosa pick-ups are on Norris yard tracks
- **THEN** the Tuscaloosa line is shown done

### Requirement: Completion detection for road trains
A road train order SHALL move to Completion Detected when every stop set-out has reached its stop, and every other car in the order, including stop pick-ups, is absent from the snapshot because it has left the map. A yard stop is reached when the car is in that yard or at an industry served by a job homed there. An industry stop is reached when the car is at that industry.

#### Scenario: Train leaves the map
- **WHEN** a new save no longer contains any of the 131 order's cars except its `BRI BRI` set-outs, which are in McCalla
- **THEN** the 131 order moves to Completion Detected

#### Scenario: Set-out carried off-map
- **WHEN** a 131 order's `BRI BRI` set-outs are absent from the snapshot
- **THEN** the order stays In Progress, with a missing-car alert on those set-outs

### Requirement: Confirm or continue
In the Completion Detected state, the user SHALL be able to confirm, which moves the order to Completed. The user SHALL alternatively be able to continue working, which returns the order to In Progress. After continuing, the system SHALL NOT detect completion again until a later snapshot changes the location of at least one of the order's cars.

#### Scenario: Confirm completion
- **WHEN** the user confirms a Completion Detected order
- **THEN** it becomes Completed, its cars are released, and it appears in history

#### Scenario: Continue working
- **WHEN** the user chooses continue on a Completion Detected order
- **THEN** it returns to In Progress and is not re-detected on an identical next snapshot

### Requirement: Manual completion and cancellation
The user SHALL be able to mark an In Progress order Completed manually. The user SHALL be able to cancel an In Progress order. Cancelling releases its cars back to generation.

#### Scenario: Cancel order
- **WHEN** the user cancels an In Progress order
- **THEN** it becomes Cancelled, and its cars can appear in newly generated orders

### Requirement: Missing car alert
A car is missing when it is absent from a snapshot and the order still expects it on the map. That covers any car in a local job order, and a road train set-out before it reaches its stop. The order SHALL be flagged with the missing car, and SHALL NOT be completed automatically while the car is missing.

#### Scenario: Car vanishes
- **WHEN** a set-out car in an AB10 order is absent from the latest snapshot
- **THEN** the AB10 order shows a missing-car alert and remains In Progress

### Requirement: Order history
Completed and Cancelled orders SHALL be retained in a history. Each history entry SHALL record the order's kind, job, cars, final state, and both the real-world and in-game timestamps of start and finish.

#### Scenario: Review history
- **WHEN** the user opens history after completing two orders
- **THEN** both are listed with start and finish times and their car lists
