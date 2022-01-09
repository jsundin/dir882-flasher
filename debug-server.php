<?php
// php -S 0.0.0.0:8000 -d file_uploads=On -d post_max_size=100M -d upload_max_filesize=100M debug-server.php
error_reporting(E_ALL);

ob_start();
print_r($_SERVER);
print_r($_GET);
print_r($_POST);
print_r($_FILES);
error_log(ob_get_contents());
ob_end_clean();

if ($_SERVER['REQUEST_METHOD'] == "GET") {
    $fn = "index.html";
} else {
    if (!empty($_FILES) && isset($_FILES['firmware'])) {
        move_uploaded_file($_FILES['firmware']['tmp_name'], '/tmp/firmware');
        $md5 = md5_file('/tmp/firmware');
        error_log("MD5: $md5 -- " . ($md5 == "7bace23026c85c0ce02075dd256d40e3" ? "OK" : "WRONG"));
    } else {
        error_log("No file detected!!!");
    }

    $fn = "success.html";
}
echo file_get_contents($fn);
?>