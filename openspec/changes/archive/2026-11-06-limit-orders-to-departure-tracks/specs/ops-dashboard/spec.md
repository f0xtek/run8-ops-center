## ADDED Requirements

### Requirement: Departure track marks
The yards view SHALL show, for each yard, whether orders take cars only from its departure tracks or from every yard track, and SHALL let the user mark or unmark as a departure track any labelled yard track and any listed yard-kind track. A track whose state differs from the profile SHALL be shown as changed. Marks SHALL be saved in the user's settings, take effect on orders at once, and survive restarts and profile upgrades.

#### Scenario: Receiving track marked for a busy yard
- **WHEN** the user marks `NOR / R07` as a departure track
- **THEN** R07 shows as a departure track changed from the profile, and the available orders and road-train build percentages count its cars without a restart

#### Scenario: Forwarding track unmarked
- **WHEN** the user unmarks `NOR / WF01`
- **THEN** the cars on WF01 leave the available orders, and WF01 shows as changed from the profile

#### Scenario: Marks survive a profile upgrade
- **WHEN** the user has marked `NOR / R07` and the bundled profile is upgraded to a newer schema version
- **THEN** R07 is still a departure track after the upgrade

#### Scenario: Every departure track unmarked
- **WHEN** the user unmarks every Norris departure track
- **THEN** the yards view says Norris orders take cars from every yard track, and cars on any NOR track count

#### Scenario: Yard rule shown
- **WHEN** Norris has 30 departure tracks and Tuscaloosa has none
- **THEN** the yards view says Norris orders take cars from its 30 departure tracks only, and Tuscaloosa orders from every yard track
