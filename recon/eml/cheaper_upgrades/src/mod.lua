local UPGRADE_COST_FACTOR = 0.5   -- rune cost of upgrades (1.0 = vanilla); stacks with the world setting
local SETTING_MIN = 0.1           -- lowest value of the "upgrade cost" world-setting slider (vanilla 0.25)
local EXTRA_STEPS = { 0.1, 0.15, 0.2 }

-- 1. The per-type cost factors (weapons/shields 1.0, armor 0.75, rings 0.5).
local bt = game.assets.get_resources_by_type("keen::BalancingTable")[1].data
local before = string.format("%.2f/%.2f/%.2f", bt.perkCostFactorWeaponsShield, bt.perkCostFactorArmor, bt.perkCostFactorRings)
bt.perkCostFactorWeaponsShield = bt.perkCostFactorWeaponsShield * UPGRADE_COST_FACTOR
bt.perkCostFactorArmor = bt.perkCostFactorArmor * UPGRADE_COST_FACTOR
bt.perkCostFactorRings = bt.perkCostFactorRings * UPGRADE_COST_FACTOR
local after = string.format("%.2f/%.2f/%.2f", bt.perkCostFactorWeaponsShield, bt.perkCostFactorArmor, bt.perkCostFactorRings)

-- 2. Let the world setting (perkCostFactor, 0.25..2.0, Relaxed 0.5) go lower,
--    and add the steps to its slider (ascending list).
local presets = game.assets.get_resources_by_type("keen::GameSettingsPresetsResource")[1].data
presets.minValues.perkCostFactor = SETTING_MIN
local added = 0
for _, r in ipairs(game.assets.get_resources_by_type("keen::FbUiBundle")) do
  local ok, ds = pcall(function() return r.data.difficultySettings end)
  if ok and ds ~= nil then
    local steps = ds.settingValues.weaponUpgradeCost.steps
    for i = #EXTRA_STEPS, 1, -1 do
      local v = EXTRA_STEPS[i]
      if v >= SETTING_MIN - 1e-6 and v < steps[1] - 1e-6 then
        table.insert(steps, 1, v); added = added + 1
      end
    end
  end
end
print("cheaper_upgrades: cost factors " .. before .. " -> " .. after .. "; setting min " .. SETTING_MIN .. ", slider steps added " .. added)
