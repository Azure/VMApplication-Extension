package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	vmextensionhelper "github.com/Azure/VMApplication-Extension/VmExtensionHelper"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
)

const (
	dataDirectoryName   = "./data/"
	cannotReadFileName  = "./data/457.proposed"
	emptyFileName       = "./data/myemptyfile"
	unparseableFileName = "./data/cannotparseme"
)

type mockOSDependencies struct {
}

func (*mockOSDependencies) stat(name string) (os.FileInfo, error) {
	return nil, errors.New("Something weird happened")
}

func (*mockOSDependencies) readfile(name string) ([]byte, error) {
	if strings.Contains(cannotReadFileName, name) {
		return nil, errors.New("This file cannot be bothered")
	}

	return ioutil.ReadFile(name)
}

func (*mockOSDependencies) writefile(filename string, data []byte, perm os.FileMode) error {
	return errors.New("The file has writer's block")
}

func (*mockOSDependencies) removefile(name string) error {
	return errors.New("This file is disinclined to acquiesce to your request")
}

func Test_getProposedFileNumber_NoSuffix(t *testing.T) {
	_, isProposed := getProposedFileNumber("yaba")
	require.False(t, isProposed)
}

func Test_getProposedFileNumber_NoDot(t *testing.T) {
	_, isProposed := getProposedFileNumber("0proposed")
	require.False(t, isProposed)
}

func Test_getProposedFileNumber_MultipleDots(t *testing.T) {
	_, isProposed := getProposedFileNumber("0.1.proposed")
	require.False(t, isProposed)
}

func Test_getProposedFileNumber_Valid(t *testing.T) {
	fileNumber, isProposed := getProposedFileNumber("14.proposed")
	require.True(t, isProposed)
	require.Equal(t, 14, fileNumber)
}

func Test_getProposedFileNumber_Hex(t *testing.T) {
	_, isProposed := getProposedFileNumber("1a.proposed")
	require.False(t, isProposed)
}

func Test_getProposedFileNumber_TooLarge(t *testing.T) {
	_, isProposed := getProposedFileNumber("4294967296335332323434323.proposed")
	require.False(t, isProposed)
}

func Test_getProposedFileNumber_EmptyNumber(t *testing.T) {
	_, isProposed := getProposedFileNumber(".proposed")
	require.False(t, isProposed)
}

func Test_getProposedFileNumber_Empty(t *testing.T) {
	_, isProposed := getProposedFileNumber("")
	require.False(t, isProposed)
}

func Test_getProposedFileNumber_Negative(t *testing.T) {
	fileNumber, isProposed := getProposedFileNumber("-2.proposed")
	require.True(t, isProposed)
	require.Equal(t, -2, fileNumber)
}

func Test_resolvePackageState_installInstallFirstHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "install", "2.0.0")
	second := getPackage("second", "install", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, first, chosen)
	require.Equal(t, operationInstall, chosen.Operation)
}

func Test_resolvePackageState_installInstallEqual(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "install", "1.0.0")
	second := getPackage("second", "install", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, first, chosen)
	require.Equal(t, operationInstall, chosen.Operation)
}

func Test_resolvePackageState_installInstallSecondHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "install", "1.0.0")
	second := getPackage("second", "install", "2.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, second, chosen)
	require.Equal(t, operationInstall, chosen.Operation)
}

func Test_resolvePackageState_installUpdateFirstHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "install", "2.0.0")
	second := getPackage("second", "update", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, first, chosen)
	require.Equal(t, operationInstall, chosen.Operation)
}

func Test_resolvePackageState_installUpdateEqual(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "install", "1.0.0")
	second := getPackage("second", "update", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, first, chosen)
	require.Equal(t, operationInstall, chosen.Operation)
}

func Test_resolvePackageState_installUpdateSecondHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "install", "1.0.0")
	second := getPackage("second", "update", "2.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, second, chosen)
	require.Equal(t, operationInstall, chosen.Operation)
}

func Test_resolvePackageState_installRemoveFirstHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "install", "2.0.0")
	second := getPackage("second", "remove", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, second, chosen)
	require.Equal(t, operationRemove, chosen.Operation)
}

func Test_resolvePackageState_installRemoveEqual(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "install", "1.0.0")
	second := getPackage("second", "remove", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, second, chosen)
	require.Equal(t, operationRemove, chosen.Operation)
}

func Test_resolvePackageState_installRemoveSecondHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "install", "1.0.0")
	second := getPackage("second", "remove", "2.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, second, chosen)
	require.Equal(t, operationRemove, chosen.Operation)
}

func Test_resolvePackageState_updateInstallFirstHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "update", "2.0.0")
	second := getPackage("second", "install", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, "first", chosen.Name)
	require.Equal(t, "2.0.0", chosen.Version)
	require.Equal(t, operationInstall, chosen.Operation)
}

func Test_resolvePackageState_updateInstallEqual(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "update", "1.0.0")
	second := getPackage("second", "install", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, "first", chosen.Name)
	require.Equal(t, "1.0.0", chosen.Version)
	require.Equal(t, operationInstall, chosen.Operation)
}

func Test_resolvePackageState_updateInstallSecondHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "update", "1.0.0")
	second := getPackage("second", "install", "2.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, second, chosen)
	require.Equal(t, operationInstall, chosen.Operation)
}

func Test_resolvePackageState_updateUpdateFirstHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "update", "2.0.0")
	second := getPackage("second", "update", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, first, chosen)
	require.Equal(t, operationUpdate, chosen.Operation)
}

func Test_resolvePackageState_updateUpdateEqual(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "update", "1.0.0")
	second := getPackage("second", "update", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, first, chosen)
	require.Equal(t, operationUpdate, chosen.Operation)
}

func Test_resolvePackageState_updateUpdateSecondHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "update", "1.0.0")
	second := getPackage("second", "update", "2.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, second, chosen)
	require.Equal(t, operationUpdate, chosen.Operation)
}

func Test_resolvePackageState_updateRemoveFirstHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "update", "2.0.0")
	second := getPackage("second", "remove", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, second, chosen)
	require.Equal(t, operationRemove, chosen.Operation)
}

func Test_resolvePackageState_updateRemoveEqual(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "update", "1.0.0")
	second := getPackage("second", "remove", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, second, chosen)
	require.Equal(t, operationRemove, chosen.Operation)
}

func Test_resolvePackageState_updateRemoveSecondHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "update", "2.0.0")
	second := getPackage("second", "remove", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, second, chosen)
	require.Equal(t, operationRemove, chosen.Operation)
}

func Test_resolvePackageState_removeInstallFirstHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "remove", "2.0.0")
	second := getPackage("second", "install", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, first, chosen)
	require.Equal(t, operationRemove, chosen.Operation)
}

func Test_resolvePackageState_removeInstallEqual(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "remove", "1.0.0")
	second := getPackage("second", "install", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, first, chosen)
	require.Equal(t, operationRemove, chosen.Operation)
}

func Test_resolvePackageState_removeInstallSecondHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "remove", "1.0.0")
	second := getPackage("second", "install", "2.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, second, chosen)
	require.Equal(t, operationInstall, chosen.Operation)
}

func Test_resolvePackageState_removeUpdateFirstHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "remove", "2.0.0")
	second := getPackage("second", "update", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, first, chosen)
	require.Equal(t, operationRemove, chosen.Operation)
}

func Test_resolvePackageState_removeUpdateEqual(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "remove", "1.0.0")
	second := getPackage("second", "update", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, first, chosen)
	require.Equal(t, operationRemove, chosen.Operation)
}

func Test_resolvePackageState_removeUpdateSecondHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "remove", "1.0.0")
	second := getPackage("second", "update", "2.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, first, chosen)
	require.Equal(t, operationRemove, chosen.Operation)
}

func Test_resolvePackageState_removeRemoveFirstHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "remove", "2.0.0")
	second := getPackage("second", "remove", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, second, chosen)
	require.Equal(t, operationRemove, chosen.Operation)
}

func Test_resolvePackageState_removeRemoveEqual(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "remove", "2.0.0")
	second := getPackage("second", "remove", "1.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, second, chosen)
	require.Equal(t, operationRemove, chosen.Operation)
}

func Test_resolvePackageState_removeRemoveSecondHigher(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	first := getPackage("first", "remove", "1.0.0")
	second := getPackage("second", "remove", "2.0.0")
	chosen := resolvePackageState(ctx, first, second)
	require.Equal(t, second, chosen)
	require.Equal(t, operationRemove, chosen.Operation)
}

func Test_compareVersions_singleDigitDifferent(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cmp := compareVersions(ctx, "1", "5")
	require.Equal(t, -1, cmp)
}

func Test_compareVersions_singleDigitSame(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cmp := compareVersions(ctx, "12", "12")
	require.Equal(t, 0, cmp)
}

func Test_compareVersions_firstIsEmpty(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cmp := compareVersions(ctx, "", "5")
	require.Equal(t, -1, cmp)
}

func Test_compareVersions_secondIsEmpty(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cmp := compareVersions(ctx, "1", "")
	require.Equal(t, 1, cmp)
}

func Test_compareVersions_bothAreEmpty(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cmp := compareVersions(ctx, "", "")
	require.Equal(t, 0, cmp)
}

func Test_compareVersions_longValueSame(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cmp := compareVersions(ctx, "3.1.4.15.9265.35897.933", "3.1.4.15.9265.35897.933")
	require.Equal(t, 0, cmp)
}

func Test_compareVersions_nonAlphabeticCompare(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cmp := compareVersions(ctx, "1.17.3", "1.8.1")
	require.Equal(t, 1, cmp)
}

func Test_compareVersions_dotDifferenceFirstLonger(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cmp := compareVersions(ctx, "1.17.3.2", "1.17.3")
	require.Equal(t, 1, cmp)
}

func Test_compareVersions_dotDifferenceSecondLonger(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cmp := compareVersions(ctx, "1.17.3", "1.17.3.2")
	require.Equal(t, -1, cmp)
}

func Test_compareVersions_cannotConvertFirst(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cmp := compareVersions(ctx, "1.1b.3", "1.8.1")
	require.Equal(t, -1, cmp)
}

func Test_compareVersions_cannotConvertSecond(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cmp := compareVersions(ctx, "1.17.3", "1.8e.1")
	require.Equal(t, -1, cmp)
}

func Test_compareVersions_trailingDotFirst(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cmp := compareVersions(ctx, "1.8.", "1.8.1")
	require.Equal(t, -1, cmp)
}

func Test_compareVersions_trailingDotSecond(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	cmp := compareVersions(ctx, "1.8.3", "1.8.")
	require.Equal(t, 1, cmp)
}

func Test_findPackageInSlice_NoPackages(t *testing.T) {
	packages := make([]vmPackage, 3)
	pos, foundPackage := findPackageInSlice("yaba", packages)
	require.Equal(t, -1, pos)
	require.Nil(t, foundPackage)
}

func Test_findPackageInSlice_NotFound(t *testing.T) {
	packages := make([]vmPackage, 3)
	packages[0] = *getPackage("flipper", "install", "1.0.0")
	packages[1] = *getPackage("iggy", "install", "1.0.0")
	packages[2] = *getPackage("yarbaflop", "install", "1.0.0")
	pos, foundPackage := findPackageInSlice("yaba", packages)
	require.Equal(t, -1, pos)
	require.Nil(t, foundPackage)
}

func Test_findPackageInSlice_FoundFirst(t *testing.T) {
	packages := make([]vmPackage, 3)
	packages[0] = *getPackage("flipper", "install", "1.0.0")
	packages[1] = *getPackage("iggy", "install", "1.0.0")
	packages[2] = *getPackage("yarbaflop", "install", "1.0.0")
	pos, foundPackage := findPackageInSlice("flipper", packages)
	require.Equal(t, 0, pos)
	require.Equal(t, packages[0].Name, foundPackage.Name)
}

func Test_findPackageInSlice_FoundLast(t *testing.T) {
	packages := make([]vmPackage, 3)
	packages[0] = *getPackage("flipper", "install", "1.0.0")
	packages[1] = *getPackage("iggy", "install", "1.0.0")
	packages[2] = *getPackage("yarbaflop", "install", "1.0.0")
	pos, foundPackage := findPackageInSlice("yarbaflop", packages)
	require.Equal(t, 2, pos)
	require.Equal(t, packages[2].Name, foundPackage.Name)
}

func Test_getPackageStateFromFile_cannotRead(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	_, err := getPackageStateFromFile(ctx, "./data/florgasnork")
	require.Error(t, err)
}

func Test_getPackageStateFromFile_fileEmpty(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	createEmptyFile(t)
	ps, err := getPackageStateFromFile(ctx, emptyFileName)
	require.NoError(t, err)
	require.NotNil(t, ps)
	require.Equal(t, 0, len(ps.Packages))
}

func Test_getPackageStateFromFile_cannotParse(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	createUnparseableFile(t)
	_, err := getPackageStateFromFile(ctx, unparseableFileName)
	require.Error(t, err)
}

func Test_getPackageStateFromFile_validFile(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	mypackage := *getPackage("flipper", "install", "1.0.0")
	data := vmPackageData{Packages: []vmPackage{mypackage}}
	createPackageFile(t, &data, "validfile")

	r, err := getPackageStateFromFile(ctx, path.Join(dataDirectoryName, "validfile"))
	require.NoError(t, err)
	require.Equal(t, 1, len(r.Packages))
	require.Equal(t, "flipper", r.Packages[0].Name)
}

func Test_getProposedPackageState_noFiles(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	cleanDataDirectory(t)

	packageData := getProposedPackageState(ctx, ext)
	require.Nil(t, packageData)
}

func Test_getProposedPackageState_noProposedFiles(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	cleanDataDirectory(t)
	createUnparseableFile(t)
	createEmptyFile(t)

	packageData := getProposedPackageState(ctx, ext)
	require.Nil(t, packageData)
}

func Test_getProposedPackageState_couldntReadFile(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	cleanDataDirectory(t)
	writeProposalFile(t, 457, "yaba", "install", "1.0.0")
	writeProposalFile(t, 2, "snorble", "install", "1.0.0")
	osDependency = &mockOSDependencies{}
	defer resetOSDependency()

	packageData := getProposedPackageState(ctx, ext)
	require.NotNil(t, packageData)
	require.Equal(t, 1, len(packageData.Packages))
	require.Equal(t, "snorble", packageData.Packages[0].Name)
}

func Test_getProposedPackageState_invalidFile(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	cleanDataDirectory(t)
	createUnparseableFileWithName(t, "./data/1.proposed")
	writeProposalFile(t, 2, "yorble", "install", "1.0.0")

	packageData := getProposedPackageState(ctx, ext)
	require.NotNil(t, packageData)
	require.Equal(t, 1, len(packageData.Packages))
	require.Equal(t, "yorble", packageData.Packages[0].Name)
}

func Test_getProposedPackageState_noPackagesFile(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	cleanDataDirectory(t)
	data := vmPackageData{}
	createPackageFile(t, &data, "3.proposal")
	writeProposalFile(t, 2, "floopa", "install", "1.0.0")

	packageData := getProposedPackageState(ctx, ext)
	require.NotNil(t, packageData)
	require.Equal(t, 1, len(packageData.Packages))
	require.Equal(t, "floopa", packageData.Packages[0].Name)
}

func Test_getProposedPackageState_emptyFile(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	cleanDataDirectory(t)
	createEmptyFileWithName(t, "./data/1.proposed")
	writeProposalFile(t, 2, "ooga", "install", "1.0.0")

	packageData := getProposedPackageState(ctx, ext)
	require.NotNil(t, packageData)
	require.Equal(t, 1, len(packageData.Packages))
	require.Equal(t, "ooga", packageData.Packages[0].Name)
}

func Test_getProposedPackageState_duplicatePackageState(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	cleanDataDirectory(t)
	writeProposalFile(t, 1, "yaba", "install", "1.0.0")
	writeProposalFile(t, 2, "yaba", "update", "2.0.0")
	writeProposalFile(t, 3, "yaba", "remove", "1.0.0")

	packageData := getProposedPackageState(ctx, ext)
	require.NotNil(t, packageData)
	require.Equal(t, 1, len(packageData.Packages))
	require.Equal(t, "remove", packageData.Packages[0].Operation)
}

func Test_getProposedPackageState_normalCase(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	cleanDataDirectory(t)
	writeProposalFile(t, 1, "yaba", "install", "1.0.0")
	writeProposalFile(t, 2, "floopa", "update", "1.0.0")
	writeProposalFile(t, 3, "zoomp", "remove", "1.0.0")

	packageData := getProposedPackageState(ctx, ext)
	require.NotNil(t, packageData)
	require.Equal(t, 3, len(packageData.Packages))
}

func Test_getCurrentPackageState_error(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	osDependency = &mockOSDependencies{}
	defer resetOSDependency()

	pkg, err := getCurrentPackageState(ctx, ext)
	require.Error(t, err)
	require.Nil(t, pkg)
}

func Test_getCurrentPackageState_doesntExist(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	cleanDataDirectory(t)

	pkg, err := getCurrentPackageState(ctx, ext)
	require.NoError(t, err)
	require.Nil(t, pkg)
}

func Test_getCurrentPackageState_valid(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))

	myPackage := *getPackage("floopa", "update", "1.0.0")
	data := vmPackageData{Packages: []vmPackage{myPackage}}
	createPackageFile(t, &data, currentPackageStateFileName)

	pkg, err := getCurrentPackageState(ctx, ext)
	require.NoError(t, err)
	require.NotNil(t, pkg)
	require.Equal(t, 1, len(pkg.Packages))
}

func Test_resolveFromPackageStates_noConflicts(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	source := vmPackageData{Packages: []vmPackage{
		*getPackage("zoopa", "update", "1.0.0"),
		*getPackage("loopa", "update", "1.0.0"),
	}}
	current := vmPackageData{Packages: []vmPackage{
		*getPackage("flip", "update", "1.0.0"),
		*getPackage("flop", "update", "1.0.0"),
	}}

	anyCallbackCalled := false
	packageLost := func(ctx log.Logger, ext *vmextensionhelper.VMExtension, losingPackage *vmPackage) {
		anyCallbackCalled = true
	}

	resolveFromPackageStates(ctx, ext, &source, &current, collectionResolveCallback(packageLost), collectionResolveCallback(packageLost))
	require.False(t, anyCallbackCalled)
	require.Equal(t, 2, len(source.Packages))
	require.Equal(t, "zoopa", source.Packages[0].Name)
	require.Equal(t, "loopa", source.Packages[1].Name)
}

func Test_resolveFromPackageStates_requestedWins(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	source := vmPackageData{Packages: []vmPackage{
		*getPackage("superapp", "install", "2.0.0"),
		*getPackage("megaapp", "remove", "1.0.0"),
	}}
	current := vmPackageData{Packages: []vmPackage{
		*getPackage("superapp", "update", "1.0.0"),
		*getPackage("megaapp", "update", "1.0.0"),
	}}

	sourceCallbackCount := 0
	sourceLost := func(ctx log.Logger, ext *vmextensionhelper.VMExtension, losingPackage *vmPackage) {
		sourceCallbackCount++
	}

	currentCallbackCount := 0
	currentLost := func(ctx log.Logger, ext *vmextensionhelper.VMExtension, losingPackage *vmPackage) {
		currentCallbackCount++
	}

	resolveFromPackageStates(ctx, ext, &source, &current, collectionResolveCallback(sourceLost), collectionResolveCallback(currentLost))
	require.Equal(t, 0, sourceCallbackCount)
	require.Equal(t, 2, currentCallbackCount)
	require.Equal(t, 2, len(source.Packages))
	require.Equal(t, "install", source.Packages[0].Operation)
	require.Equal(t, "remove", source.Packages[1].Operation)
}

func Test_resolveFromPackageStates_currentWins(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	source := vmPackageData{Packages: []vmPackage{
		*getPackage("superapp", "update", "1.0.0"),
		*getPackage("megaapp", "remove", "1.0.0"),
	}}
	current := vmPackageData{Packages: []vmPackage{
		*getPackage("superapp", "install", "2.0.0"),
		*getPackage("megaapp", "update", "1.0.0"),
	}}

	sourceCallbackCount := 0
	sourceLost := func(ctx log.Logger, ext *vmextensionhelper.VMExtension, losingPackage *vmPackage) {
		sourceCallbackCount++
	}

	currentCallbackCount := 0
	currentLost := func(ctx log.Logger, ext *vmextensionhelper.VMExtension, losingPackage *vmPackage) {
		currentCallbackCount++
	}

	resolveFromPackageStates(ctx, ext, &source, &current, collectionResolveCallback(sourceLost), collectionResolveCallback(currentLost))
	require.Equal(t, 1, sourceCallbackCount)
	require.Equal(t, 1, currentCallbackCount)
	require.Equal(t, 1, len(source.Packages))
	require.Equal(t, "megaapp", source.Packages[0].Name)
	require.Equal(t, "remove", source.Packages[0].Operation)
}

func Test_resolveFromPackageStates_currentWinsForAll(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	source := vmPackageData{Packages: []vmPackage{
		*getPackage("superapp", "update", "1.0.0"),
		*getPackage("megaapp", "update", "1.0.0"),
	}}
	current := vmPackageData{Packages: []vmPackage{
		*getPackage("superapp", "install", "2.0.0"),
		*getPackage("megaapp", "remove", "1.0.0"),
	}}

	sourceCallbackCount := 0
	sourceLost := func(ctx log.Logger, ext *vmextensionhelper.VMExtension, losingPackage *vmPackage) {
		sourceCallbackCount++
	}

	currentCallbackCount := 0
	currentLost := func(ctx log.Logger, ext *vmextensionhelper.VMExtension, losingPackage *vmPackage) {
		currentCallbackCount++
	}

	resolveFromPackageStates(ctx, ext, &source, &current, collectionResolveCallback(sourceLost), collectionResolveCallback(currentLost))
	require.Equal(t, 2, sourceCallbackCount)
	require.Equal(t, 0, currentCallbackCount)
	require.Equal(t, 0, len(source.Packages))
}

func Test_writeProposedPackages_cannotWriteFile(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	osDependency = &mockOSDependencies{}
	defer resetOSDependency()

	data := vmPackageData{Packages: []vmPackage{
		*getPackage("superapp", "update", "1.0.0"),
	}}
	err := writeProposedPackages(ctx, ext, &data, 0)
	require.Error(t, err)
}

func Test_writeProposedPackages_noPackages(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	cleanDataDirectory(t)

	data := vmPackageData{Packages: []vmPackage{}}
	err := writeProposedPackages(ctx, ext, &data, 0)
	require.NoError(t, err)
}

func Test_writeProposedPackages_onePackage(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	cleanDataDirectory(t)

	data := vmPackageData{Packages: []vmPackage{
		*getPackage("superapp", "update", "1.0.0"),
	}}
	err := writeProposedPackages(ctx, ext, &data, 5)
	require.NoError(t, err)

	readData := getProposedPackageState(ctx, ext)
	require.Equal(t, 1, len(readData.Packages))
	require.Equal(t, "superapp", readData.Packages[0].Name)
}

func Test_writeProposedPackages_severalPackages(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	cleanDataDirectory(t)

	data := vmPackageData{Packages: []vmPackage{
		*getPackage("scuba", "update", "1.0.0"),
		*getPackage("floopa", "update", "1.0.0"),
		*getPackage("goopa", "update", "1.0.0"),
		*getPackage("loopa", "update", "1.0.0"),
	}}
	err := writeProposedPackages(ctx, ext, &data, 5)
	require.NoError(t, err)

	readData := getProposedPackageState(ctx, ext)
	require.Equal(t, 4, len(readData.Packages))
	require.Equal(t, 5, readData.Packages[0].ProposedFileNumber)
	require.Equal(t, 8, readData.Packages[3].ProposedFileNumber)
}

func Test_getNextProposedPackage_noPackages(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	cleanDataDirectory(t)

	nextPackage := getNextProposedPackage(ctx, ext)
	require.Nil(t, nextPackage)
}

func Test_getNextProposedPackage_startsAtZero(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	cleanDataDirectory(t)

	data := vmPackageData{Packages: []vmPackage{
		*getPackage("scuba", "update", "1.0.0"),
		*getPackage("floopa", "update", "1.0.0"),
		*getPackage("goopa", "update", "1.0.0"),
		*getPackage("loopa", "update", "1.0.0"),
	}}
	err := writeProposedPackages(ctx, ext, &data, 0)
	require.NoError(t, err)

	nextPackage := getNextProposedPackage(ctx, ext)
	require.NotNil(t, nextPackage)
	require.Equal(t, "scuba", nextPackage.Name)
}

func Test_getNextProposedPackage_higherNumber(t *testing.T) {
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	cleanDataDirectory(t)

	data := vmPackageData{Packages: []vmPackage{
		*getPackage("scuba", "update", "1.0.0"),
		*getPackage("floopa", "update", "1.0.0"),
		*getPackage("goopa", "update", "1.0.0"),
		*getPackage("loopa", "update", "1.0.0"),
	}}
	err := writeProposedPackages(ctx, ext, &data, 723)
	require.NoError(t, err)

	nextPackage := getNextProposedPackage(ctx, ext)
	require.NotNil(t, nextPackage)
	require.Equal(t, "scuba", nextPackage.Name)
}

func Test_getPackageStatePlan_allPackagesRemovedByProposed(t *testing.T) {
	cleanDataDirectory(t)
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	requested := vmPackageData{Packages: []vmPackage{
		*getPackage("iggy", "update", "2.0.0"),
		*getPackage("piggy", "update", "2.0.0"),
	}}
	ext := createTestVMExtension(requested)

	proposed := vmPackageData{Packages: []vmPackage{
		*getPackage("iggy", "update", "3.0.0"),
		*getPackage("piggy", "update", "3.0.0"),
	}}
	err := writeProposedPackages(ctx, ext, &proposed, 0)
	require.NoError(t, err)

	current := vmPackageData{Packages: []vmPackage{
		*getPackage("iggy", "update", "1.0.0"),
		*getPackage("piggy", "update", "1.0.0"),
	}}
	createPackageFile(t, &current, currentPackageStateFileName)

	requiresChanges, hasProposedState, err := getPackageStatePlan(ctx, ext, &requested)
	require.False(t, requiresChanges)
	require.True(t, hasProposedState)
	require.NoError(t, err)

	// We should still have the two proposed files
	nextPackage := getNextProposedPackage(ctx, ext)
	require.NotNil(t, nextPackage)
	require.Equal(t, "3.0.0", nextPackage.Version)
	markProposedPackageFinished(ctx, ext, nextPackage)

	nextPackage = getNextProposedPackage(ctx, ext)
	require.NotNil(t, nextPackage)
	require.Equal(t, "3.0.0", nextPackage.Version)
	markProposedPackageFinished(ctx, ext, nextPackage)
}

func Test_getPackageStatePlan_allPackagesRemovedByCurrent(t *testing.T) {
	cleanDataDirectory(t)
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	requested := vmPackageData{Packages: []vmPackage{
		*getPackage("iggy", "update", "2.0.0"),
		*getPackage("piggy", "update", "2.0.0"),
	}}
	ext := createTestVMExtension(requested)

	current := vmPackageData{Packages: []vmPackage{
		*getPackage("iggy", "update", "4.0.0"),
		*getPackage("piggy", "update", "4.0.0"),
	}}
	createPackageFile(t, &current, currentPackageStateFileName)

	requiresChanges, hasProposedState, err := getPackageStatePlan(ctx, ext, &requested)
	require.False(t, requiresChanges)
	require.False(t, hasProposedState)
	require.NoError(t, err)

	// We should have no proposed files
	nextPackage := getNextProposedPackage(ctx, ext)
	require.Nil(t, nextPackage)
}

func Test_getPackageStatePlan_removeProposedPackage(t *testing.T) {
	cleanDataDirectory(t)
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	requested := vmPackageData{Packages: []vmPackage{
		*getPackage("iggy", "update", "3.0.0"),
		*getPackage("piggy", "update", "3.0.0"),
	}}
	ext := createTestVMExtension(requested)

	proposed := vmPackageData{Packages: []vmPackage{
		*getPackage("piggy", "update", "2.0.0"),
	}}
	err := writeProposedPackages(ctx, ext, &proposed, 0)
	require.NoError(t, err)

	current := vmPackageData{Packages: []vmPackage{
		*getPackage("iggy", "install", "1.0.0"),
		*getPackage("piggy", "install", "1.0.0"),
	}}
	createPackageFile(t, &current, currentPackageStateFileName)

	requiresChanges, hasProposedState, err := getPackageStatePlan(ctx, ext, &requested)
	require.True(t, requiresChanges)
	require.True(t, hasProposedState)
	require.NoError(t, err)

	// The proposed file will be deleted
	nextPackage := getNextProposedPackage(ctx, ext)
	require.NotNil(t, nextPackage)
	require.Equal(t, "3.0.0", nextPackage.Version)
	markProposedPackageFinished(ctx, ext, nextPackage)

	nextPackage = getNextProposedPackage(ctx, ext)
	require.NotNil(t, nextPackage)
	require.Equal(t, "3.0.0", nextPackage.Version)
	markProposedPackageFinished(ctx, ext, nextPackage)
}

func Test_getPackageStatePlan_cannotRemoveProposedPackage(t *testing.T) {
	cleanDataDirectory(t)
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	requested := vmPackageData{Packages: []vmPackage{
		*getPackage("iggy", "update", "3.0.0"),
		*getPackage("piggy", "update", "3.0.0"),
	}}
	ext := createTestVMExtension(requested)

	proposed := vmPackageData{Packages: []vmPackage{
		*getPackage("piggy", "update", "2.0.0"),
	}}
	err := writeProposedPackages(ctx, ext, &proposed, 0)
	require.NoError(t, err)

	current := vmPackageData{Packages: []vmPackage{
		*getPackage("iggy", "install", "1.0.0"),
		*getPackage("piggy", "install", "1.0.0"),
	}}
	createPackageFile(t, &current, currentPackageStateFileName)

	osDependency = &mockOSDependencies{}
	defer resetOSDependency()

	_, _, err = getPackageStatePlan(ctx, ext, &requested)
	require.Error(t, err)
}

func Test_getPackageStatePlan_cannotWriteProposedPackages(t *testing.T) {
	cleanDataDirectory(t)
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	requested := vmPackageData{Packages: []vmPackage{
		*getPackage("iggy", "update", "3.0.0"),
		*getPackage("piggy", "update", "3.0.0"),
	}}
	ext := createTestVMExtension(requested)

	current := vmPackageData{Packages: []vmPackage{
		*getPackage("iggy", "install", "1.0.0"),
		*getPackage("piggy", "install", "1.0.0"),
	}}
	createPackageFile(t, &current, currentPackageStateFileName)

	osDependency = &mockOSDependencies{}
	defer resetOSDependency()

	_, _, err := getPackageStatePlan(ctx, ext, &requested)
	require.Error(t, err)
}

func Test_getPackageStatePlan_addToProposedPackages(t *testing.T) {
	cleanDataDirectory(t)
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	requested := vmPackageData{Packages: []vmPackage{
		*getPackage("iggy", "install", "3.0.0"),
	}}
	ext := createTestVMExtension(requested)

	proposed := vmPackageData{Packages: []vmPackage{
		*getPackage("piggy", "update", "3.0.0"),
	}}
	err := writeProposedPackages(ctx, ext, &proposed, 0)
	require.NoError(t, err)

	current := vmPackageData{Packages: []vmPackage{
		*getPackage("piggy", "install", "1.0.0"),
	}}
	createPackageFile(t, &current, currentPackageStateFileName)

	requiresChanges, hasProposedState, err := getPackageStatePlan(ctx, ext, &requested)
	require.True(t, requiresChanges)
	require.True(t, hasProposedState)
	require.NoError(t, err)

	// We'll now have two proposed files
	nextPackage := getNextProposedPackage(ctx, ext)
	require.NotNil(t, nextPackage)
	require.Equal(t, "3.0.0", nextPackage.Version)
	markProposedPackageFinished(ctx, ext, nextPackage)

	nextPackage = getNextProposedPackage(ctx, ext)
	require.NotNil(t, nextPackage)
	require.Equal(t, "3.0.0", nextPackage.Version)
	markProposedPackageFinished(ctx, ext, nextPackage)
}

func Test_getPackageStatePlan_noProposedPackages(t *testing.T) {
	cleanDataDirectory(t)
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	requested := vmPackageData{Packages: []vmPackage{
		*getPackage("iggy", "install", "3.0.0"),
	}}
	ext := createTestVMExtension(requested)

	current := vmPackageData{Packages: []vmPackage{
		*getPackage("piggy", "install", "1.0.0"),
	}}
	createPackageFile(t, &current, currentPackageStateFileName)

	requiresChanges, hasProposedState, err := getPackageStatePlan(ctx, ext, &requested)
	require.True(t, requiresChanges)
	require.False(t, hasProposedState)
	require.NoError(t, err)

	// We'll have one proposed file
	nextPackage := getNextProposedPackage(ctx, ext)
	require.NotNil(t, nextPackage)
	require.Equal(t, "3.0.0", nextPackage.Version)
	markProposedPackageFinished(ctx, ext, nextPackage)

	nextPackage = getNextProposedPackage(ctx, ext)
	require.Nil(t, nextPackage)
}

func Test_getPackageStatePlan_noCurrentPackages(t *testing.T) {
	cleanDataDirectory(t)
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	requested := vmPackageData{Packages: []vmPackage{
		*getPackage("piggy", "update", "3.0.0"),
	}}
	ext := createTestVMExtension(requested)

	proposed := vmPackageData{Packages: []vmPackage{
		*getPackage("piggy", "update", "2.0.0"),
	}}
	err := writeProposedPackages(ctx, ext, &proposed, 0)
	require.NoError(t, err)

	requiresChanges, hasProposedState, err := getPackageStatePlan(ctx, ext, &requested)
	require.True(t, requiresChanges)
	require.True(t, hasProposedState)
	require.NoError(t, err)

	nextPackage := getNextProposedPackage(ctx, ext)
	require.NotNil(t, nextPackage)
	require.Equal(t, "3.0.0", nextPackage.Version)
	markProposedPackageFinished(ctx, ext, nextPackage)

	nextPackage = getNextProposedPackage(ctx, ext)
	require.Nil(t, nextPackage)
}

func Test_getPackageStatePlan_noCurrentOrProposedPackages(t *testing.T) {
	cleanDataDirectory(t)
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	requested := vmPackageData{Packages: []vmPackage{
		*getPackage("piggy", "update", "3.0.0"),
	}}
	ext := createTestVMExtension(requested)

	requiresChanges, hasProposedState, err := getPackageStatePlan(ctx, ext, &requested)
	require.True(t, requiresChanges)
	require.False(t, hasProposedState)
	require.NoError(t, err)

	nextPackage := getNextProposedPackage(ctx, ext)
	require.NotNil(t, nextPackage)
	require.Equal(t, "3.0.0", nextPackage.Version)
	markProposedPackageFinished(ctx, ext, nextPackage)

	nextPackage = getNextProposedPackage(ctx, ext)
	require.Nil(t, nextPackage)
}

func Test_getPackageStatePlan_noRequestedPackages(t *testing.T) {
	cleanDataDirectory(t)
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	requested := vmPackageData{Packages: []vmPackage{}}
	ext := createTestVMExtension(requested)

	requiresChanges, hasProposedState, err := getPackageStatePlan(ctx, ext, &requested)
	require.False(t, requiresChanges)
	require.False(t, hasProposedState)
	require.NoError(t, err)
}

func Test_markProposedPackageFinished_doesntExist(t *testing.T) {
	cleanDataDirectory(t)
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	pkg := getPackage("piggy", "install", "1.0.0")
	pkg.ProposedFileNumber = 5

	err := markProposedPackageFinished(ctx, ext, pkg)
	require.Error(t, err)
}

func Test_markProposedPackageFinished_error(t *testing.T) {
	cleanDataDirectory(t)
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))
	pkg := getPackage("piggy", "update", "3.0.0")
	pkg.ProposedFileNumber = 5
	proposed := vmPackageData{Packages: []vmPackage{*pkg}}
	err := writeProposedPackages(ctx, ext, &proposed, 0)
	require.NoError(t, err)

	osDependency = &mockOSDependencies{}
	defer resetOSDependency()

	err = markProposedPackageFinished(ctx, ext, pkg)
	require.Error(t, err)
}

func Test_markProposedPackageFinished_valid(t *testing.T) {
	cleanDataDirectory(t)
	ctx := log.NewSyncLogger(log.NewLogfmtLogger(os.Stdout))
	ext := createTestVMExtension(createSettings(nil))

	proposed := vmPackageData{Packages: []vmPackage{
		*getPackage("ziggy", "update", "3.0.0"),
		*getPackage("piggy", "update", "3.0.0"),
	}}
	err := writeProposedPackages(ctx, ext, &proposed, 0)
	require.NoError(t, err)

	nextPackage := getNextProposedPackage(ctx, ext)
	require.NotNil(t, nextPackage)
	require.Equal(t, "ziggy", nextPackage.Name)
	markProposedPackageFinished(ctx, ext, nextPackage)

	nextPackage = getNextProposedPackage(ctx, ext)
	require.NotNil(t, nextPackage)
	require.Equal(t, "piggy", nextPackage.Name)
	markProposedPackageFinished(ctx, ext, nextPackage)

	nextPackage = getNextProposedPackage(ctx, ext)
	require.Nil(t, nextPackage)
}

func resetOSDependency() {
	osDependency = &osDependenciesImpl{}
}

func writeProposalFile(t *testing.T, number int, packageName string, operation string, version string) {
	myPackage := *getPackage(packageName, operation, version)
	data := vmPackageData{Packages: []vmPackage{myPackage}}
	fileName := strconv.Itoa(number) + "." + proposedPackageStateSuffix
	createPackageFile(t, &data, fileName)
}

func cleanDataDirectory(t *testing.T) {
	createDataDirectoryIfNecessary()

	// Open the directory and read all its files.
	dirRead, err := os.Open(dataDirectoryName)
	require.NoError(t, err, "os.Open failed")
	dirFiles, err := dirRead.Readdir(0)
	require.NoError(t, err, "Readdir failed")

	// Loop over the directory's files.
	for index := range dirFiles {
		fileToDelete := dirFiles[index]
		fullPath := path.Join(dataDirectoryName, fileToDelete.Name())
		err = os.Remove(fullPath)
		require.NoError(t, err, "os.Remove failed")
	}
}

func createPackageFile(t *testing.T, data *vmPackageData, fileName string) {
	createDataDirectoryIfNecessary()
	filePath := path.Join(dataDirectoryName, fileName)
	b, err := json.Marshal(data)
	require.NoError(t, err)
	err = ioutil.WriteFile(filePath, b, 0)
	require.NoError(t, err)
}

func createEmptyFile(t *testing.T) {
	createEmptyFileWithName(t, emptyFileName)
}

func createUnparseableFile(t *testing.T) {
	createUnparseableFileWithName(t, unparseableFileName)
}

func createEmptyFileWithName(t *testing.T, fileName string) {
	createDataDirectoryIfNecessary()
	ef, err := os.Create(fileName)
	require.NoError(t, err)
	ef.Close()
}

func createUnparseableFileWithName(t *testing.T, fileName string) {
	createDataDirectoryIfNecessary()
	gibberish := "} yargle = flarg, [but) this {} will not.=] parse"
	b := []byte(gibberish)
	err := ioutil.WriteFile(fileName, b, 0)
	require.NoError(t, err)
}

func createDataDirectoryIfNecessary() {
	_ = os.Mkdir(dataDirectoryName, os.ModePerm)
}

func getPackage(name string, operation string, version string) *vmPackage {
	return &vmPackage{
		Name:      name,
		Operation: operation,
		Version:   version,
	}
}
