package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const connStr = "/ as sysdba"

func main() {
	fmt.Println("=============================================================================")
	fmt.Println("                      YashanDB AWR Report Generator                          ")
	fmt.Println("=============================================================================")

	// Step 1: Fetch snapshots
	fmt.Println("\n[1/3] Fetching snapshots from WRM$_SNAPSHOT...")
	
	listSql := `
SET FEEDBACK OFF;
SET HEADING ON;
COLUMN BEGIN_TIME FORMAT a20;
COLUMN END_TIME FORMAT a20;

SELECT DBID, 
       INSTANCE_NUMBER AS INST, 
       SNAP_ID, 
       TO_CHAR(BEGIN_INTERVAL_TIME, 'YYYY-MM-DD HH24:MI:SS') AS BEGIN_TIME, 
       TO_CHAR(END_INTERVAL_TIME, 'YYYY-MM-DD HH24:MI:SS') AS END_TIME 
FROM WRM$_SNAPSHOT 
ORDER BY SNAP_ID ASC;
`
	
	cmdList := exec.Command("yasql", "-s", connStr)
	cmdList.Stdin = strings.NewReader(listSql)
	listRaw, err := cmdList.CombinedOutput()

	// --- 关键改进：如果连接失败（err != nil），直接退出 ---
	if err != nil || strings.Contains(string(listRaw), "YAS-00402") || strings.Contains(string(listRaw), "YASQL-00007") {
		fmt.Printf("\n[FATAL ERROR] Failed to connect to YashanDB:\n%s\n", string(listRaw))
		fmt.Println("Please check if the database is running and your permissions.")
		os.Exit(1) // 终止程序
	}

	// 执行清洗并打印
	printCleanContent(string(listRaw))

	// Step 2: User Input
	var beginSnap, endSnap string
	fmt.Print("\nEnter BEGIN Snapshot ID: ")
	fmt.Scanln(&beginSnap)
	fmt.Print("Enter END   Snapshot ID: ")
	fmt.Scanln(&endSnap)

	if beginSnap == "" || endSnap == "" {
		fmt.Println("Error: Snapshot IDs are required.")
		os.Exit(1)
	}

	fmt.Printf("\n[2/3] Generating AWR report...")

	// Step 3: Generate AWR
	awrSql := fmt.Sprintf(`
SET FEEDBACK OFF;
SET SERVEROUTPUT ON;
DECLARE
    v_dbid NUMBER;
    v_inst NUMBER;
BEGIN
    SELECT DBID, INSTANCE_NUMBER INTO v_dbid, v_inst 
    FROM WRM$_SNAPSHOT WHERE SNAP_ID = %s AND ROWNUM = 1;

    DBMS_OUTPUT.PUT_LINE('FILENAME_METADATA:' || v_dbid || '_' || v_inst);
    DBMS_AWR.AWR_REPORT(v_dbid, v_inst, %s, %s);
END;
/
`, beginSnap, beginSnap, endSnap)

	cmdGen := exec.Command("yasql", "-s", connStr)
	cmdGen.Stdin = strings.NewReader(awrSql)
	outputBytes, err := cmdGen.CombinedOutput()
	
	// --- 再次检查连接 ---
	if err != nil {
		fmt.Printf("\n[FATAL ERROR] Generation failed:\n%s\n", string(outputBytes))
		os.Exit(1)
	}

	content := string(outputBytes)

	// Step 4: Metadata for Filename
	dbidInst := "unknown_1"
	if idx := strings.Index(content, "FILENAME_METADATA:"); idx != -1 {
		line := content[idx+len("FILENAME_METADATA:"):]
		if endIdx := strings.Index(line, "\n"); endIdx != -1 {
			dbidInst = strings.TrimSpace(line[:endIdx])
		}
	}
	outputFile := fmt.Sprintf("awr_%s_%s_%s.html", dbidInst, beginSnap, endSnap)

	// Step 5: Save HTML
	htmlStart := "<!DOCTYPE html>"
	startIndex := strings.Index(content, htmlStart)
	if startIndex == -1 {
		startIndex = strings.Index(content, "<html")
	}

	if startIndex != -1 {
		finalHtml := content[startIndex:]
		err := os.WriteFile(outputFile, []byte(finalHtml), 0644)
		if err != nil {
			fmt.Printf("\nError: Failed to write file: %v\n", err)
		} else {
			fmt.Printf("\n[3/3] Success! Report created: %s\n", outputFile)
		}
	} else {
		fmt.Println("\nError: No HTML content found in output.")
		fmt.Println("--- Captured Log ---")
		fmt.Println(content)
	}
}

func printCleanContent(raw string) {
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || 
		   strings.HasPrefix(trimmed, "SQL>") || 
		   strings.Contains(trimmed, "YashanDB") || 
		   strings.Contains(trimmed, "Connected to") || 
		   strings.Contains(trimmed, "Disconnected from") ||
		   strings.Contains(trimmed, "Release") {
			continue
		}
		fmt.Println(line)
	}
}

