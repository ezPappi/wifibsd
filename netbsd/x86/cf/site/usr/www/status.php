<?php
/*
 * Shows server operational status, for elected services.
 * Marcin Jessa <yazzy@yazzy.org>. 
 */

print "<table width='100%' border='0' cellspacing='0' padding='0'>\n";
print "<tr>";
print "<td width='90%'>";
print "<table border='0' cellspacing='0' padding='3'>\n";
$ports = array(
        "80"   => "HTTP",
        "22"   => "SSH",
        "21"   => "FTP",
        "53"   => "DNS);
asort($ports);
print "<b>Server Operational Status:</b><br>\n";
print "<hr width='300' align='left'>\n";
$border_count = 1;
foreach ($ports as $port => $name) {
  if (($border_count % 2)) { 
    $border = " bgcolor='#f8f8f8'";
  } else {
    $border = ""; 
  }
  if ($border_count > 1) { $border .= " style='border-top: 1px dashed #c8dcc8; padding: 3px'"; } 
  else { $border .= " style='padding: 3px'"; }
  $border_count++;
  $command = fsockopen("localhost",$port, $errno, $errstr, 20);
  if ($command) {
    echo "<tr><td width='277'$border>$name</td><td width='13' align='center' bgcolor='#00BF7E'></td></tr>\n";
  }
  elseif (!$command) {
    echo "<tr><td width='277'$border>$name</td><td width='13' align='center' bgcolor='#FF0000'></td></tr>\n";
  }
}
print "</table>\n";
print "</td>";
print "<td>";
print "<img src='/images/status.jpg' border='0'>";
print "</td></tr>";
print "</table>";
?>
