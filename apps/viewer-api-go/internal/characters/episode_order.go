package characters

import "strconv"

func episodeWithin(value string, upToEpisodeIndex string) bool {
	return value != "" && compareEpisodeIndex(value, upToEpisodeIndex) <= 0
}

func buildEpisodePositionMap(episodeIndexes []string) map[string]int {
	result := map[string]int{}
	for index, episodeIndex := range episodeIndexes {
		if episodeIndex != "" {
			result[episodeIndex] = index + 1
		}
	}
	return result
}

func totalEpisodesConsidered(episodeIndexes []string, upToEpisodeIndex string, episodePositionMap map[string]int) int {
	if position := episodePositionMap[upToEpisodeIndex]; position > 0 {
		return position
	}
	parsed, err := strconv.Atoi(upToEpisodeIndex)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func episodePosition(episodeIndex string, episodePositionMap map[string]int) int {
	if position := episodePositionMap[episodeIndex]; position > 0 {
		return position
	}
	parsed, err := strconv.Atoi(episodeIndex)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func compareEpisodeByOrder(left string, right string, episodePositionMap map[string]int) int {
	leftPosition := episodePositionMap[left]
	rightPosition := episodePositionMap[right]
	if leftPosition > 0 && rightPosition > 0 {
		return leftPosition - rightPosition
	}
	return compareEpisodeIndex(left, right)
}

func compareEpisodeIndex(left string, right string) int {
	leftNumber, leftErr := strconv.ParseInt(left, 10, 64)
	rightNumber, rightErr := strconv.ParseInt(right, 10, 64)
	if leftErr == nil && rightErr == nil {
		if leftNumber < rightNumber {
			return -1
		}
		if leftNumber > rightNumber {
			return 1
		}
		return 0
	}
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
