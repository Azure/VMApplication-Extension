package utils

import (
	"strconv"
	"strings"
)

// compares the version strings
// returns 0 if both are equal, returns < 0 if versionString1 < versionString2 and returns > 0 if versionString 1 > versionString2
func CompareVersion(versionString1 *string, versionString2 *string) (int, error) {
	numbersText1 := strings.Split(*versionString1, ".")
	numbersText2 := strings.Split(*versionString2, ".")

	len1 := len(numbersText1)
	len2 := len(numbersText2)

	i := 0

	for ; i < len1 && i < len2; i++ {
		num1, err := strconv.Atoi(numbersText1[i])
		if err != nil {
			return 0, err
		}

		num2, err := strconv.Atoi(numbersText2[i])
		if err != nil {
			return 0, err
		}

		if num1 == num2 {
			continue;
		} else {
			//
			return num1 - num2, nil
		}
	}

	if i == len1 && i == len2 {
		// all the individual version numbers were equal
		return 0, nil
	}

	if len1 > len2 {
		// 1.2.0.0 is same as
		remainingVersion, err := findNonZeroVersionNumber(numbersText1, i, len1)
		if err != nil {
			return 0, err
		}

		if remainingVersion == 0 {
			// 1.2.0.0 is considered the same as 1.2
			return 0, nil
		} else
		{
			return 1, nil
		}
	} else
	{
		remainingVersion, err := findNonZeroVersionNumber(numbersText2, i, len2)
		if err != nil {
			return 0, err
		}

		if remainingVersion == 0 {
			// 1.2.0.0 is considered the same as 1.2
			return 0, nil
		} else
		{
			return -1, nil
		}
	}
}

func findNonZeroVersionNumber(versionStringSlice []string, startIndex int, length int) (int, error) {
	for i := startIndex; i < length; i++ {
		num, err := strconv.Atoi(versionStringSlice[i])
		if err != nil {
			return 0, err
		}
		if num != 0 {
			return num, nil
		}
	}
	return 0, nil
}
