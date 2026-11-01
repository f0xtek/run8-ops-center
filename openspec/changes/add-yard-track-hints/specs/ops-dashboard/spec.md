# Spec Delta

## MODIFIED Requirements

### Requirement: Yards and industries view
The interface SHALL list every yard with its tracks and the cars on each. For a yard with a track list, it SHALL also show every listed track: labelled ones with the listed length next to the measured length, and unlabelled ones marked as not labelled yet. It SHALL list every industry, grouped by serving job, with its cars, service states, and occupancy against capacity for each car type.

#### Scenario: Industry occupancy
- **WHEN** HTI has 10 Two Bay Covered Hoppers and a capacity of 24
- **THEN** the industry shows 10/24 for that car type

#### Scenario: Listed track not labelled yet
- **WHEN** yard TUS lists TY06 and no section is labelled `TUS / TY06`
- **THEN** the yards view shows TY06 under Tuscaloosa as not labelled yet
