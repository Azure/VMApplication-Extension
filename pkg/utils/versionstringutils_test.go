package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestVersionStringEqual(t *testing.T) {
	vString1 := "1"
	vString2 := "1"
	result, err := CompareVersion(&vString1, &vString2)
	boolResult := AreVersionsEqual(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.EqualValues(t, 0, result, "result should be equal to 0")
	assert.True(t, boolResult, "version strings should be equal")

	vString1 = "1.0.0"
	vString2 = "1"
	result, err = CompareVersion(&vString1, &vString2)
	boolResult = AreVersionsEqual(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.EqualValues(t, 0, result, "result should be equal to 0")
	assert.True(t, boolResult, "version strings should be equal")

	vString1 = "1"
	vString2 = "1.0.0"
	result, err = CompareVersion(&vString1, &vString2)
	boolResult = AreVersionsEqual(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.EqualValues(t, 0, result, "result should be equal to 0")
	assert.True(t, boolResult, "version strings should be equal")

	vString1 = "1.2.3"
	vString2 = "1.2.3"
	result, err = CompareVersion(&vString1, &vString2)
	boolResult = AreVersionsEqual(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.EqualValues(t, 0, result, "result should be equal to 0")
	assert.True(t, boolResult, "version strings should be equal")

	vString1 = "1.2.3"
	vString2 = "1.2.3.0"
	result, err = CompareVersion(&vString1, &vString2)
	boolResult = AreVersionsEqual(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.EqualValues(t, 0, result, "result should be equal to 0")
	assert.True(t, boolResult, "version strings should be equal")

	vString1 = "1.2.3.0"
	vString2 = "1.2.3"
	result, err = CompareVersion(&vString1, &vString2)
	boolResult = AreVersionsEqual(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.EqualValues(t, 0, result, "result should be equal to 0")
	assert.True(t, boolResult, "version strings should be equal")

	vString1 = "1.4b.2"
	vString2 = "1.4b.2"
	boolResult = AreVersionsEqual(&vString1, &vString2)
	assert.True(t, boolResult, "version strings should be equal")
}

func TestAreVersionsUnequal(t *testing.T) {
	vString1 := "1.xyz.2"
	vString2 := "1.abc.2"
	boolResult := AreVersionsEqual(&vString1, &vString2)
	assert.False(t, boolResult, "version strings should be unequal")
}

func TestVersionStringGreater(t *testing.T) {
	vString1 := "1.2.3"
	vString2 := "1"
	result, err := CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.Greater(t, result, 0, "result should be greater than 0")

	vString1 = "2"
	vString2 = "1"
	result, err = CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.Greater(t, result, 0, "result should be greater than 0")

	vString1 = "1.0.1"
	vString2 = "1"
	result, err = CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.Greater(t, result, 0, "result should be greater than 0")

	vString1 = "1.3.5"
	vString2 = "1.3.4"
	result, err = CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.Greater(t, result, 0, "result should be greater than 0")

	vString1 = "1.2.21"
	vString2 = "1.2.3"
	result, err = CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.Greater(t, result, 0, "result should be greater than 0")

	vString1 = "1.3.4"
	vString2 = "1.3"
	result, err = CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.Greater(t, result, 0, "result should be greater than 0")
}

func TestVersionStringSmaller(t *testing.T) {
	vString1 := "1"
	vString2 := "1.2.3"
	result, err := CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.Greater(t, 0, result, "result should be smaller than 0")

	vString1 = "1"
	vString2 = "2"
	result, err = CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.Greater(t, 0, result, "result should be smaller than 0")

	vString1 = "1"
	vString2 = "1.0.1"
	result, err = CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.Greater(t, 0, result, "result should be smaller than 0")

	vString1 = "1.3.4"
	vString2 = "1.3.5"
	result, err = CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.Greater(t, 0, result, "result should be smaller than 0")

	vString1 = "1.2.3"
	vString2 = "1.2.21"
	result, err = CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.Greater(t, 0, result, "result should be smaller than 0")

	vString1 = "1.3"
	vString2 = "1.3.4"
	result, err = CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.Greater(t, 0, result, "result should be smaller than 0")
}
