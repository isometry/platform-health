package shared_test

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isometry/platform-health/pkg/commands/shared"
	"github.com/isometry/platform-health/pkg/provider"

	// Register mock provider for testing
	_ "github.com/isometry/platform-health/pkg/provider/mock"
)

func TestAddProviderSubcommands(t *testing.T) {
	t.Run("SubcommandCreated", func(t *testing.T) {
		parent := &cobra.Command{Use: "test"}

		shared.AddProviderSubcommands(parent, shared.ProviderSubcommandOptions{
			RunFunc: func(cmd *cobra.Command, providerType string) error {
				return nil
			},
		})

		// Should have at least the mock subcommand
		mockCmd, _, err := parent.Find([]string{"mock"})
		require.NoError(t, err)
		assert.Equal(t, "mock", mockCmd.Use)
	})

	t.Run("FlagsRegistered", func(t *testing.T) {
		parent := &cobra.Command{Use: "test"}

		shared.AddProviderSubcommands(parent, shared.ProviderSubcommandOptions{
			RunFunc: func(cmd *cobra.Command, providerType string) error {
				return nil
			},
		})

		mockCmd, _, err := parent.Find([]string{"mock"})
		require.NoError(t, err)

		// Verify provider flags are registered
		healthFlag := mockCmd.Flags().Lookup("health")
		assert.NotNil(t, healthFlag, "health flag should be registered")

		sleepFlag := mockCmd.Flags().Lookup("sleep")
		assert.NotNil(t, sleepFlag, "sleep flag should be registered")
	})

	t.Run("RequireChecks_Included", func(t *testing.T) {
		parent := &cobra.Command{Use: "test"}

		shared.AddProviderSubcommands(parent, shared.ProviderSubcommandOptions{
			RequireChecks: true,
			RunFunc: func(cmd *cobra.Command, providerType string) error {
				return nil
			},
		})

		// Mock is check-capable, should be included
		mockCmd, _, err := parent.Find([]string{"mock"})
		require.NoError(t, err)
		assert.Equal(t, "mock", mockCmd.Use)
	})

	t.Run("SetupFlagsCalled", func(t *testing.T) {
		parent := &cobra.Command{Use: "test"}
		setupFlagsCalled := false
		var receivedInstance provider.Instance

		shared.AddProviderSubcommands(parent, shared.ProviderSubcommandOptions{
			SetupFlags: func(cmd *cobra.Command, instance provider.Instance) {
				setupFlagsCalled = true
				receivedInstance = instance
				// Add a custom flag to verify this was called
				cmd.Flags().Bool("custom-flag", false, "test flag")
			},
			RunFunc: func(cmd *cobra.Command, providerType string) error {
				return nil
			},
		})

		assert.True(t, setupFlagsCalled, "SetupFlags should be called")
		assert.NotNil(t, receivedInstance, "Instance should be passed to SetupFlags")

		// Verify the custom flag was added
		mockCmd, _, err := parent.Find([]string{"mock"})
		require.NoError(t, err)
		customFlag := mockCmd.Flags().Lookup("custom-flag")
		assert.NotNil(t, customFlag, "custom flag should be registered")
	})

	t.Run("RunFuncCalled", func(t *testing.T) {
		parent := &cobra.Command{Use: "test"}
		runFuncCalled := false
		var receivedProviderType string

		shared.AddProviderSubcommands(parent, shared.ProviderSubcommandOptions{
			RunFunc: func(cmd *cobra.Command, providerType string) error {
				runFuncCalled = true
				receivedProviderType = providerType
				return nil
			},
		})

		// Execute subcommand via parent with args
		parent.SetArgs([]string{"mock"})
		err := parent.Execute()
		require.NoError(t, err)

		assert.True(t, runFuncCalled, "RunFunc should be called")
		assert.Equal(t, "mock", receivedProviderType)
	})

	t.Run("RunFuncErrorPropagated", func(t *testing.T) {
		parent := &cobra.Command{Use: "test"}
		expectedErr := errors.New("test error")

		shared.AddProviderSubcommands(parent, shared.ProviderSubcommandOptions{
			RunFunc: func(cmd *cobra.Command, providerType string) error {
				return expectedErr
			},
		})

		// Execute subcommand via parent with args
		parent.SetArgs([]string{"mock"})
		err := parent.Execute()
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestCreateAndConfigureProvider(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("health", "HEALTHY", "")
		cmd.Flags().Duration("sleep", 0, "")

		instance, err := shared.CreateAndConfigureProvider(cmd, "mock")
		require.NoError(t, err)
		assert.NotNil(t, instance)
		assert.Equal(t, "mock", instance.GetType())
		assert.True(t, provider.SupportsChecks(instance), "mock should be check-capable")
	})

	t.Run("VerifyFlagValues", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("health", "UNHEALTHY", "")
		cmd.Flags().Duration("sleep", 0, "")

		instance, err := shared.CreateAndConfigureProvider(cmd, "mock")
		require.NoError(t, err)

		// The instance should have been configured from flags
		// We can verify by calling Setup and GetHealth
		require.NoError(t, instance.Setup())
		health := instance.GetHealth(t.Context())
		assert.Equal(t, "UNHEALTHY", health.GetStatus().String())
	})

	t.Run("UnknownProvider", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}

		instance, err := shared.CreateAndConfigureProvider(cmd, "unknown-provider")
		assert.Error(t, err)
		assert.Nil(t, instance)
		assert.Contains(t, err.Error(), "not registered")
	})

	t.Run("InvalidEnumValue", func(t *testing.T) {
		// Invalid enum values should produce an error during configuration
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("health", "INVALID_STATUS", "")
		cmd.Flags().Duration("sleep", 0, "")

		_, err := shared.CreateAndConfigureProvider(cmd, "mock")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown enum value")
	})

	t.Run("ValidEnumValue", func(t *testing.T) {
		// Valid enum values like LOOP_DETECTED should work
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("health", "LOOP_DETECTED", "")
		cmd.Flags().Duration("sleep", 0, "")

		instance, err := shared.CreateAndConfigureProvider(cmd, "mock")
		require.NoError(t, err)
		require.NotNil(t, instance)

		require.NoError(t, instance.Setup())
		health := instance.GetHealth(t.Context())
		assert.Equal(t, "LOOP_DETECTED", health.GetStatus().String())
	})

	t.Run("CustomFlagValues", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("health", "HEALTHY", "")
		cmd.Flags().Duration("sleep", 100000000, "") // 100ms

		// Set the flag value
		require.NoError(t, cmd.Flags().Set("health", "UNHEALTHY"))

		instance, err := shared.CreateAndConfigureProvider(cmd, "mock")
		require.NoError(t, err)

		require.NoError(t, instance.Setup())
		health := instance.GetHealth(t.Context())
		assert.Equal(t, "UNHEALTHY", health.GetStatus().String())
	})
}

func TestProviderSubcommandShortDescription(t *testing.T) {
	parent := &cobra.Command{Use: "test"}

	shared.AddProviderSubcommands(parent, shared.ProviderSubcommandOptions{
		RunFunc: func(cmd *cobra.Command, providerType string) error {
			return nil
		},
	})

	mockCmd, _, err := parent.Find([]string{"mock"})
	require.NoError(t, err)

	// Verify short description format
	assert.Contains(t, mockCmd.Short, "mock")
	assert.Contains(t, mockCmd.Short, "provider")
}
