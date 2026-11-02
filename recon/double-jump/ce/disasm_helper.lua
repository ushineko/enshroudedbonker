-- Drives CE through files. Put it in CE's autorun/ and set DIR to a Z:\ path ending in \.
-- Each line of req.txt gets an answer in resp.txt:
--   <hexaddr> <count>            disassemble count instructions
--   W <hexaddr> <expected> <new> write 32 bits, only if the current value matches
-- It writes nothing without a W line. tools/ask.py sends requests.
local DIR = [[Z:\CHANGE\ME\]]
local REQ, RESP = DIR .. "req.txt", DIR .. "resp.txt"
local status = io.open(DIR .. "ce_status.txt", "w")
local function say(s) status:write(s .. "\n"); status:flush() end
say("autorun loaded " .. getCEVersion())
local t = createTimer(nil, false); t.Interval = 300
t.OnTimer = function()
  if getOpenedProcessID() == 0 then
    local ok = openProcess("enshrouded.exe")
    say("openProcess " .. tostring(ok) .. " pid=" .. tostring(getOpenedProcessID()))
    return
  end
  local f = io.open(REQ, "r"); if not f then return end
  local txt = f:read("*a"); f:close(); os.remove(REQ)
  local out = io.open(RESP .. ".tmp", "w")
  for line in txt:gmatch("[^\r\n]+") do
    local wa, we, wn = line:match("^W%s+(%x+)%s+(%x+)%s+(%x+)$")
    if wa then
      local addr, want, val = tonumber(wa, 16), tonumber(we, 16), tonumber(wn, 16)
      local cur = readInteger(addr)
      if cur == want then
        local ok = writeInteger(addr, val)
        out:write(string.format("W %X was %X wrote %X ok=%s now %X\n", addr, cur, val, tostring(ok), readInteger(addr) or 0))
      else
        out:write(string.format("W %X REFUSED: current %X, expected %X\n", addr, cur or 0, want))
      end
    end
    local a, n = line:match("^(%x+)%s+(%d+)$")
    if a then
      local addr = tonumber(a, 16)
      out:write("; " .. a .. " +" .. n .. "\n")
      for _ = 1, tonumber(n) do
        local s = disassemble(addr); if not s then break end
        out:write(string.format("%X  %s", addr, s) .. "\n")
        addr = addr + getInstructionSize(addr)
      end
    end
  end
  out:close(); os.rename(RESP .. ".tmp", RESP)
end
t.Enabled = true
