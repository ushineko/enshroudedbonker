local WANT = { Dungeons = true, Locations = true }
local reg = game.assets.get_resources_by_type("keen::MapMarkerRegistryResource")[1].data
local n = 0
for _, m in ipairs(reg.mapMarkers.values) do
  if WANT[m.sortingCategory] and not m.isFastTravelDestination then
    m.isFastTravelDestination = true; n = n + 1
  end
end
print("fast_travel_markers: enabled " .. n)
