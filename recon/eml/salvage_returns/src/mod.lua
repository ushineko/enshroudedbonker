local RECYCLING_MAX = 2.0   -- top of the "upgrade recycling" world-setting slider (vanilla 1.0 = 100%)
local EXTRA_STEPS = { 2.0, 1.75, 1.5, 1.25 }
local SALVAGE_MULTIPLIER = 2  -- runes from salvaging gear (BalancingTable trashStart/trashEnd)

-- Salvaging an upgraded item refunds runes: (runes spent on upgrades) x the
-- world setting perkUpgradeRecyclingFactor (Default 0.5, Relaxed 1.0, max 1.0).
-- Raise the allowed maximum and add the higher steps to the settings slider.
local presets = game.assets.get_resources_by_type("keen::GameSettingsPresetsResource")[1].data
presets.maxValues.perkUpgradeRecyclingFactor = RECYCLING_MAX

local added = 0
for _, r in ipairs(game.assets.get_resources_by_type("keen::FbUiBundle")) do
  local ok, ds = pcall(function() return r.data.difficultySettings end)
  if ok and ds ~= nil then
    local steps = ds.settingValues.perkUpgradeRecyclingFactor.steps   -- descending
    for i = #EXTRA_STEPS, 1, -1 do
      local v = EXTRA_STEPS[i]
      if v <= RECYCLING_MAX and v > steps[1] + 1e-6 then
        table.insert(steps, 1, v); added = added + 1
      end
    end
  end
end
-- Salvaging unwanted gear: BalancingTable.trashStart / trashEnd (vanilla 5 and 20)
-- look like the low and high end of the rune payout. Unconfirmed; checked in game.
local bt = game.assets.get_resources_by_type("keen::BalancingTable")[1].data
local t0, t1 = bt.trashStart, bt.trashEnd
bt.trashStart = math.floor(t0 * SALVAGE_MULTIPLIER)
bt.trashEnd = math.floor(t1 * SALVAGE_MULTIPLIER)
print("salvage_returns: trash " .. t0 .. "-" .. t1 .. " -> " .. bt.trashStart .. "-" .. bt.trashEnd)
print("salvage_returns: recycling max " .. RECYCLING_MAX .. ", slider steps added " .. added)
