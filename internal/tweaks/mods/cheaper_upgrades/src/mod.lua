-- @tweak name="Cheaper upgrades" group="Crafting"
-- @desc Cuts the rune cost of upgrading weapons, armor and rings at the vendors,
-- @desc and lets the world's upgrade-cost setting go below 0.25x. The two multiply.
-- @param UPGRADE_COST_FACTOR label="Upgrade cost" min=0.05 max=1 step=0.05 default=0.5 kind=float
-- @param SETTING_MIN label="Lowest upgrade-cost setting" min=0.05 max=0.25 step=0.05 default=0.1 kind=float
--
-- Part of enshroudedbonker (MIT). A plain EML mod: drop the folder into
-- upstream EML's mods/ and it runs as is.

local UPGRADE_COST_FACTOR = 0.5
local SETTING_MIN = 0.1

-- 1. The per-type cost factors (weapons/shields 1.0, armor 0.75, rings 0.5).
local bt = game.assets.get_resources_by_type("keen::BalancingTable")[1].data
bt.perkCostFactorWeaponsShield = bt.perkCostFactorWeaponsShield * UPGRADE_COST_FACTOR
bt.perkCostFactorArmor = bt.perkCostFactorArmor * UPGRADE_COST_FACTOR
bt.perkCostFactorRings = bt.perkCostFactorRings * UPGRADE_COST_FACTOR

-- 2. Let the world setting (perkCostFactor, 0.25..2.0, Relaxed 0.5) go lower,
--    and add the steps to its slider, whose steps run from low to high.
local presets = game.assets.get_resources_by_type("keen::GameSettingsPresetsResource")[1].data
presets.minValues.perkCostFactor = SETTING_MIN
local added = 0
for _, r in ipairs(game.assets.get_resources_by_type("keen::FbUiBundle")) do
  local ok, ds = pcall(function() return r.data.difficultySettings end)
  if ok and ds ~= nil then
    local steps = ds.settingValues.weaponUpgradeCost.steps
    local v = 0.2
    while v >= SETTING_MIN - 1e-6 do
      if v < steps[1] - 1e-6 then
        table.insert(steps, 1, v)
        added = added + 1
      end
      v = v - 0.05
    end
  end
end
print("cheaper_upgrades: cost x" .. UPGRADE_COST_FACTOR .. "; setting down to " .. SETTING_MIN ..
  " (" .. added .. " slider steps)")
