package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestVersionStringEqual(t *testing.T){


	vString1 := "1"
	vString2 := "1"
	result, err := CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.EqualValues(t, result, 0, "result should be equal to 0")

	vString1 = "1.0.0"
	vString2 = "1"
	result, err = CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.EqualValues(t, result, 0, "rresult should be equal to 0")

	vString1 = "1"
	vString2 = "1.0.0"
	result, err = CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.EqualValues(t, result, 0, "result should be equal to 0")

	vString1 = "1.2.3"
	vString2 = "1.2.3"
	result, err = CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.EqualValues(t, result, 0, "result should be equal to 0")

	vString1 = "1.2.3"
	vString2 = "1.2.3.0"
	result, err = CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.EqualValues(t, result, 0, "result should be equal to 0")


	vString1 = "1.2.3.0"
	vString2 = "1.2.3"
	result, err = CompareVersion(&vString1, &vString2)
	assert.NoError(t, err, "Error not expected")
	assert.EqualValues(t, result, 0, "result should be equal to 0")
}

func TestVersionStringGreater(t *testing.T){
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
}

func TestVersionStringSmaller(t *testing.T){
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
}

