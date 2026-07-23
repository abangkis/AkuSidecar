package domain

import "testing"

func TestUpdatePolicyAcceptsOnlySupportedAuthorityCombinations(t *testing.T) {
	valid := []UpdatePolicy{
		{Trigger: UpdateTriggerOnboarding, Delivery: UpdateDeliveryVisible, BudgetAuthority: BudgetAuthorityUser},
		{Trigger: UpdateTriggerScheduler, Delivery: UpdateDeliveryPrepared, BudgetAuthority: BudgetAuthorityAutomatic},
		{Trigger: UpdateTriggerUser, Delivery: UpdateDeliveryVisible, BudgetAuthority: BudgetAuthorityUser},
		{Trigger: UpdateTriggerUser, Delivery: UpdateDeliveryPrepared, BudgetAuthority: BudgetAuthorityAutomatic},
	}
	for _, policy := range valid {
		if err := policy.Validate(); err != nil {
			t.Fatalf("valid policy %+v: %v", policy, err)
		}
	}
	invalid := []UpdatePolicy{
		{Trigger: UpdateTriggerOnboarding, Delivery: UpdateDeliveryPrepared, BudgetAuthority: BudgetAuthorityAutomatic},
		{Trigger: UpdateTriggerScheduler, Delivery: UpdateDeliveryVisible, BudgetAuthority: BudgetAuthorityUser},
		{Trigger: UpdateTriggerUser, Delivery: UpdateDeliveryPrepared, BudgetAuthority: BudgetAuthorityUser},
	}
	for _, policy := range invalid {
		if err := policy.Validate(); err == nil {
			t.Fatalf("invalid policy accepted: %+v", policy)
		}
	}
}
